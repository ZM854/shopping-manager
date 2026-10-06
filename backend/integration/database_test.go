//go:build integration

package integration_test

import (
	"context"
	"testing"
	"time"

	"github.com/ZM854/shopping-manager/backend/internal/testutil/testdb"
)

func TestDatabaseSchemasAreIsolated(t *testing.T) {
	t.Parallel()
	first, second := testdb.New(t), testdb.New(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := first.Exec(ctx, "CREATE TABLE isolation_probe (id BIGINT PRIMARY KEY); INSERT INTO isolation_probe VALUES (1)"); err != nil {
		t.Fatal(err)
	}
	var table *string
	if err := second.QueryRow(ctx, "SELECT to_regclass('isolation_probe')::text").Scan(&table); err != nil {
		t.Fatal(err)
	}
	if table != nil {
		t.Fatal("Вторая схема видит данные первой")
	}
}

func TestFreshDatabaseMigrationsAndRollback(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	testdb.ApplyMigrations(t, pool, 0, 3)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	// Проверяем текущую структуру после всех трёх миграций.
	var count int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM products WHERE user_id = 1").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("Тестовая схема содержит чужие данные")
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	if _, err := tx.Exec(ctx, "INSERT INTO users (name, email, password_hash) VALUES ('Тест', 'rollback@example.test', 'test-only')"); err != nil {
		t.Fatal(err)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM users").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("Откат транзакции оставил тестовые данные")
	}
}

func TestSchemaCleanupDoesNotRemoveOtherSchema(t *testing.T) {
	observer := testdb.New(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var childSchema string
	t.Run("temporary schema", func(t *testing.T) {
		pool := testdb.New(t)
		if err := pool.QueryRow(ctx, "SELECT current_schema()").Scan(&childSchema); err != nil {
			t.Fatal(err)
		}
		// Cleanup дочернего теста выполнится до продолжения родителя.
	})
	var exists bool
	if err := observer.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM pg_namespace WHERE nspname = $1)", childSchema).Scan(&exists); err != nil {
		t.Fatal(err)
	}
	if exists {
		t.Fatal("Cleanup оставил дочернюю тестовую схему")
	}
	// В оставшейся схеме проверяем последовательное применение диапазонов.
	testdb.ApplyMigrations(t, observer, 0, 1)
	testdb.ApplyMigrations(t, observer, 1, 3)
	var count int
	if err := observer.QueryRow(ctx, "SELECT count(*) FROM users WHERE name = 'Тест'").Scan(&count); err != nil {
		t.Fatal(err)
	}
}
