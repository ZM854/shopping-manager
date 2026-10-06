// Package testdb создаёт отдельную схему для каждого интеграционного теста.
// Подключение допустимо только к выделенной БД с выделенным тестовым пользователем.
package testdb

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const DatabaseName = "shopping_manager_test"
const UserName = "shopping_manager_test"

// ParseConfig не использует рабочий .env и не выводит строку подключения в ошибках.
func ParseConfig(databaseURL string) (*pgxpool.Config, error) {
	if databaseURL == "" {
		return nil, errors.New("TEST_DATABASE_URL не задан: запустите отдельную тестовую PostgreSQL; интеграционные тесты не пропускаются")
	}
	if !strings.HasPrefix(databaseURL, "postgres://") && !strings.HasPrefix(databaseURL, "postgresql://") {
		return nil, errors.New("TEST_DATABASE_URL должен быть PostgreSQL URL с явным тестовым пользователем и базой")
	}
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, errors.New("TEST_DATABASE_URL содержит некорректную конфигурацию")
	}
	if cfg.ConnConfig.Database != DatabaseName || cfg.ConnConfig.User != UserName {
		return nil, errors.New("разрешены только база shopping_manager_test и пользователь shopping_manager_test; рабочая БД запрещена")
	}
	cfg.ConnConfig.ConnectTimeout = 5 * time.Second
	return cfg, nil
}

func New(t testing.TB) *pgxpool.Pool {
	t.Helper()
	cfg, err := ParseConfig(os.Getenv("TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	admin, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal("Не удалось создать подключение к тестовой PostgreSQL")
	}
	var database, user string
	if err := admin.QueryRow(ctx, "SELECT current_database(), current_user").Scan(&database, &user); err != nil {
		admin.Close()
		t.Fatal("Тестовая PostgreSQL недоступна; проверьте запуск отдельного окружения")
	}
	if database != DatabaseName || user != UserName {
		admin.Close()
		t.Fatal("Фактические база и пользователь не соответствуют тестовому окружению")
	}
	random := make([]byte, 16)
	if _, err := rand.Read(random); err != nil {
		admin.Close()
		t.Fatal("Не удалось сформировать имя тестовой схемы")
	}
	schema := "sm_test_" + hex.EncodeToString(random)
	identifier := pgx.Identifier{schema}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+identifier); err != nil {
		admin.Close()
		t.Fatal("Не удалось создать изолированную тестовую схему")
	}
	var pool *pgxpool.Pool
	t.Cleanup(func() {
		if pool != nil {
			pool.Close()
		}
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		// Имя сгенерировано здесь; удаляется только принадлежащая этому тесту схема.
		if _, err := admin.Exec(cleanupCtx, "DROP SCHEMA "+identifier+" CASCADE"); err != nil {
			t.Error("Не удалось очистить созданную тестом схему")
		}
		admin.Close()
	})
	isolated := cfg.Copy()
	// public не входит в search_path: SQL тестов не видит таблицы приложения.
	isolated.ConnConfig.RuntimeParams["search_path"] = identifier
	pool, err = pgxpool.NewWithConfig(ctx, isolated)
	if err != nil {
		t.Fatal("Не удалось создать пул изолированной тестовой схемы")
	}
	if err := pool.Ping(ctx); err != nil {
		t.Fatal("Не удалось подключиться к изолированной тестовой схеме")
	}
	return pool
}

// ApplyMigrations применяет только up-файлы (from, to] в одной транзакции.
// Вызов от 0 до 3 готовит старую БД, от 3 до 4 проверит её обновление.
func ApplyMigrations(t testing.TB, pool *pgxpool.Pool, from, to int) {
	t.Helper()
	if from < 0 || to <= from {
		t.Fatal("Некорректный диапазон тестовых миграций")
	}
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("Не удалось определить каталог миграций")
	}
	directory := filepath.Join(filepath.Dir(file), "..", "..", "..", "migrations")
	paths, err := filepath.Glob(filepath.Join(directory, "*.up.sql"))
	if err != nil {
		t.Fatal("Не удалось прочитать каталог миграций")
	}
	sort.Strings(paths)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal("Не удалось начать транзакцию миграций")
	}
	defer tx.Rollback(context.Background())
	next := from + 1
	for _, path := range paths {
		prefix, _, _ := strings.Cut(filepath.Base(path), "_")
		version, err := strconv.Atoi(prefix)
		if err != nil {
			t.Fatal("Некорректное имя миграции")
		}
		if version <= from || version > to {
			continue
		}
		if version != next {
			t.Fatalf("Ожидалась миграция %d, найдена %d", next, version)
		}
		sql, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("Не удалось прочитать миграцию %d", version)
		}
		if _, err := tx.Exec(ctx, string(sql)); err != nil {
			t.Fatal(fmt.Errorf("миграция %d не выполнена: %w", version, err))
		}
		next++
	}
	if next != to+1 {
		t.Fatalf("Не найдены все миграции до версии %d", to)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal("Не удалось зафиксировать тестовые миграции")
	}
}
