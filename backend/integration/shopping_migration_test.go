//go:build integration

package integration_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/ZM854/shopping-manager/backend/internal/testutil/testdb"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

func shoppingMigrationSQL(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("Cannot locate migration")
	}
	sql, err := os.ReadFile(filepath.Join(filepath.Dir(file), "..", "migrations", "000004_shopping_lists.up.sql"))
	if err != nil {
		t.Fatal(err)
	}
	return string(sql)
}

func migrationExec(t *testing.T, pool *pgxpool.Pool, sql string, args ...any) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := pool.Exec(ctx, sql, args...); err != nil {
		t.Fatal(err)
	}
}

func migrationUser(t *testing.T, pool *pgxpool.Pool, email string) int64 {
	t.Helper()
	var id int64
	if err := pool.QueryRow(context.Background(),
		`INSERT INTO users (email, name, password_hash) VALUES ($1, 'Тест', 'test-hash') RETURNING id`, email).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func migrationScalar(t *testing.T, pool *pgxpool.Pool, sql string, args ...any) string {
	t.Helper()
	var result string
	if err := pool.QueryRow(context.Background(), sql, args...).Scan(&result); err != nil {
		t.Fatal(err)
	}
	return result
}

func TestShoppingMigrationFreshDatabase(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	testdb.ApplyMigrations(t, pool, 0, 4)
	if got := migrationScalar(t, pool, `SELECT count(*)::text FROM shopping_lists`); got != "0" {
		t.Fatalf("Fresh lists = %s", got)
	}
	if got := migrationScalar(t, pool, `SELECT count(*)::text FROM shopping_items`); got != "0" {
		t.Fatalf("Fresh items = %s", got)
	}
	if got := migrationScalar(t, pool, `SELECT count(*)::text FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = 'shopping_items' AND column_name = 'user_id'`); got != "0" {
		t.Fatal("Redundant owner column remains")
	}
	if got := migrationScalar(t, pool, `SELECT data_type || ':' || is_nullable || ':' || coalesce(numeric_scale::text, 'unrestricted') FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = 'shopping_items' AND column_name = 'quantity'`); got != "numeric:YES:unrestricted" {
		t.Fatalf("Quantity schema = %s", got)
	}
	for _, index := range []struct{ name, columns string }{
		{"shopping_lists_owner_id_id_idx", "(owner_id, id)"},
		{"shopping_items_list_id_id_idx", "(list_id, id)"},
	} {
		got := migrationScalar(t, pool, `SELECT indexdef FROM pg_indexes WHERE schemaname = current_schema() AND indexname = $1`, index.name)
		if !strings.Contains(got, index.columns) {
			t.Fatalf("Unexpected index: %s", got)
		}
	}
	first := migrationUser(t, pool, "new@example.test")
	if got := migrationScalar(t, pool, `SELECT count(*)::text FROM shopping_lists WHERE owner_id = $1`, first); got != "0" {
		t.Fatal("New user must start without lists")
	}
	listID := migrationScalar(t, pool, `INSERT INTO shopping_lists (owner_id, name) VALUES ($1, 'Покупки') RETURNING id::text`, first)
	migrationExec(t, pool, `INSERT INTO shopping_lists (owner_id, name) VALUES ($1, 'Покупки')`, first)
	var created, updated time.Time
	if err := pool.QueryRow(context.Background(), `SELECT created_at, updated_at FROM shopping_lists WHERE id = $1`, listID).Scan(&created, &updated); err != nil {
		t.Fatal(err)
	}
	if created.IsZero() || !created.Equal(updated) {
		t.Fatal("Timestamp defaults lost")
	}
	if got := migrationScalar(t, pool, `INSERT INTO shopping_items (list_id, name) VALUES ($1, 'Молоко') RETURNING (quantity IS NULL AND unit = '' AND NOT is_marked)::text`, listID); got != "true" {
		t.Fatal("Item defaults lost")
	}
}

func TestShoppingMigrationPreservesLegacyData(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	testdb.ApplyMigrations(t, pool, 0, 3)
	first := migrationUser(t, pool, "first@example.test")
	second := migrationUser(t, pool, "second@example.test")
	empty := migrationUser(t, pool, "empty@example.test")
	migrationExec(t, pool, `INSERT INTO refresh_tokens (user_id, token_hash) VALUES ($1, 'test-token-hash')`, first)
	migrationExec(t, pool, `INSERT INTO products (user_id, name, quantity, unit, is_marked) VALUES
        ($1, ' Молоко 🥛 ', 0.1, ' л ', true), ($2, 'Соль', 0.001, 'кг', false),
        ($1, 'Мука', 999999999.999, '', false), ($2, 'Сахар', 2.675, 'кг', true),
        ($1, 'Крупа', 12.345, 'кг', false)`, first, second)
	migrationExec(t, pool, `SELECT setval('products_id_seq', 9007199254740993)`)
	migrationExec(t, pool, `INSERT INTO products (user_id, name, quantity, unit) VALUES ($1, 'Большой ID', 1.125, 'шт')`, first)
	before := migrationScalar(t, pool, `SELECT jsonb_agg(jsonb_build_array(id, user_id, name, quantity::text, unit, is_marked) ORDER BY id)::text FROM products`)
	testdb.ApplyMigrations(t, pool, 3, 4)
	after := migrationScalar(t, pool, `SELECT jsonb_agg(jsonb_build_array(item.id, list.owner_id, item.name, item.quantity::text, item.unit, item.is_marked) ORDER BY item.id)::text FROM shopping_items item JOIN shopping_lists list ON list.id = item.list_id`)
	if before != after {
		t.Fatalf("Legacy data changed:\nbefore %s\nafter  %s", before, after)
	}
	if got := migrationScalar(t, pool, `SELECT count(*)::text FROM shopping_lists WHERE name = 'Покупки' AND created_at = updated_at`); got != "3" {
		t.Fatalf("Default lists = %s", got)
	}
	if got := migrationScalar(t, pool, `SELECT count(*)::text FROM shopping_items item JOIN shopping_lists list ON list.id = item.list_id WHERE list.owner_id = $1`, empty); got != "0" {
		t.Fatal("Empty user's list contains foreign items")
	}
	if got := migrationScalar(t, pool, `SELECT count(*)::text FROM users WHERE password_hash = 'test-hash'`); got != "3" {
		t.Fatal("Users changed")
	}
	if got := migrationScalar(t, pool, `SELECT token_hash FROM refresh_tokens WHERE user_id = $1`, first); got != "test-token-hash" {
		t.Fatal("Auth data changed")
	}
	if got := migrationScalar(t, pool, `INSERT INTO shopping_items (list_id, name, quantity) SELECT id, 'Следующая позиция', 1 FROM shopping_lists WHERE owner_id = $1 RETURNING id::text`, second); got != "9007199254740995" {
		t.Fatalf("Item sequence reset: %s", got)
	}
	if got := migrationScalar(t, pool, `INSERT INTO shopping_lists (owner_id, name) VALUES ($1, 'Второй') RETURNING id::text`, first); got != "4" {
		t.Fatalf("List sequence reset: %s", got)
	}
}

func TestShoppingMigrationRejectsInvalidLegacyRowsAtomically(t *testing.T) {
	cases := []struct{ name, field, value, category string }{
		{"empty name", "name", "", "names"},
		{"spaces name", "name", " \u00a0\u2003\u3000", "names"},
		{"control name", "name", "a\nb", "names"},
		{"C1 control name", "name", "a\u0085b", "names"},
		{"long name", "name", strings.Repeat("я", 201), "names"},
		{"long unit", "unit", strings.Repeat("я", 33), "units"},
		{"control unit", "unit", "a\tb", "units"},
		{"zero", "quantity", "0", "quantities"},
		{"negative", "quantity", "-1", "quantities"},
		{"NaN", "quantity", "NaN", "quantities"},
		{"positive infinity", "quantity", "Infinity", "quantities"},
		{"negative infinity", "quantity", "-Infinity", "quantities"},
		{"below minimum", "quantity", "0.0001", "quantities"},
		{"above maximum", "quantity", "1000000000", "quantities"},
		{"too precise", "quantity", "1.2345", "quantities"},
		{"near integer", "quantity", "1.0000000000000002", "quantities"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pool := testdb.New(t)
			testdb.ApplyMigrations(t, pool, 0, 3)
			owner := migrationUser(t, pool, "legacy@example.test")
			migrationExec(t, pool, `INSERT INTO products (user_id, name, quantity, unit) VALUES ($1, 'Valid', 1, 'шт'), ($1, 'Invalid', 1, 'шт')`, owner)
			// field выбирается только из фиксированной таблицы выше.
			cast := ""
			if tc.field == "quantity" {
				cast = "::double precision"
			}
			migrationExec(t, pool, "UPDATE products SET "+tc.field+" = $1"+cast+" WHERE id = 2", tc.value)
			before := migrationScalar(t, pool, `SELECT jsonb_agg(to_jsonb(products) ORDER BY id)::text FROM products`)
			conn, err := pool.Acquire(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			// Миграция обязана отбраковать лишнюю точность даже при такой настройке.
			if _, err := conn.Exec(context.Background(), `SET extra_float_digits = -3`); err != nil {
				conn.Release()
				t.Fatal(err)
			}
			_, err = conn.Exec(context.Background(), shoppingMigrationSQL(t))
			migrationErr := err
			if _, err := conn.Exec(context.Background(), `SET extra_float_digits = 3`); err != nil {
				conn.Release()
				t.Fatal(err)
			}
			conn.Release()
			var pgErr *pgconn.PgError
			if !errors.As(migrationErr, &pgErr) || pgErr.Code != "23514" || !strings.Contains(pgErr.Message, "invalid product "+tc.category+" (1 rows)") {
				t.Fatalf("Expected diagnostic for %s, got %v", tc.category, migrationErr)
			}
			after := migrationScalar(t, pool, `SELECT jsonb_agg(to_jsonb(products) ORDER BY id)::text FROM products`)
			if before != after {
				t.Fatal("Failed migration changed original rows")
			}
			if got := migrationScalar(t, pool, `SELECT (to_regclass('shopping_lists') IS NULL AND to_regclass('shopping_items') IS NULL AND to_regclass('products_id_seq') IS NOT NULL)::text`); got != "true" {
				t.Fatal("Failed migration left a partial schema")
			}
			migrationExec(t, pool, `UPDATE products SET name = 'Исправлено', unit = '', quantity = 1 WHERE id = 2`)
			testdb.ApplyMigrations(t, pool, 3, 4)
			if got := migrationScalar(t, pool, `SELECT count(*)::text FROM shopping_items`); got != "2" {
				t.Fatal("Retry after explicit correction lost rows")
			}
		})
	}
}

func TestShoppingMigrationRollsBackLateFailure(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	testdb.ApplyMigrations(t, pool, 0, 3)
	owner := migrationUser(t, pool, "legacy@example.test")
	migrationExec(t, pool, `INSERT INTO products (user_id, name, quantity, unit) VALUES ($1, 'Сохранить', 0.1, '')`, owner)
	migrationExec(t, pool, `CREATE TABLE shopping_items (sentinel text)`)
	if _, err := pool.Exec(context.Background(), shoppingMigrationSQL(t)); err == nil {
		t.Fatal("Expected rename collision")
	}
	if got := migrationScalar(t, pool, `SELECT (to_regclass('shopping_lists') IS NULL AND to_regclass('shopping_lists_id_seq') IS NULL AND to_regclass('products') IS NOT NULL)::text`); got != "true" {
		t.Fatal("DDL before failure was not rolled back")
	}
	if got := migrationScalar(t, pool, `SELECT name FROM products`); got != "Сохранить" {
		t.Fatal("Late failure changed data")
	}
}

func TestShoppingMigrationRespectsOuterTransaction(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	testdb.ApplyMigrations(t, pool, 0, 3)
	owner := migrationUser(t, pool, "transaction@example.test")
	migrationExec(t, pool, `INSERT INTO products (user_id, name, quantity, unit) VALUES ($1, 'Сохранить', 0.1, '')`, owner)
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, shoppingMigrationSQL(t)); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM shopping_items`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("Migration within transaction: count=%d, err=%v", count, err)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	if got := migrationScalar(t, pool, `SELECT (to_regclass('shopping_lists') IS NULL AND to_regclass('shopping_items') IS NULL)::text`); got != "true" {
		t.Fatal("Migration committed the outer transaction")
	}
	if got := migrationScalar(t, pool, `SELECT quantity::text FROM products`); got != "0.1" {
		t.Fatal("Outer rollback lost legacy data")
	}
	// Тот же файл успешно работает без внешней транзакции, как отдельная команда SQL.
	migrationExec(t, pool, shoppingMigrationSQL(t))
	if got := migrationScalar(t, pool, `SELECT quantity::text FROM shopping_items`); got != "0.1" {
		t.Fatal("Standalone migration lost quantity")
	}
}

func TestShoppingSchemaConstraintsAndCascades(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	testdb.ApplyMigrations(t, pool, 0, 4)
	first := migrationUser(t, pool, "first@example.test")
	second := migrationUser(t, pool, "second@example.test")
	listA := migrationScalar(t, pool, `INSERT INTO shopping_lists (owner_id, name) VALUES ($1, 'А') RETURNING id::text`, first)
	listB := migrationScalar(t, pool, `INSERT INTO shopping_lists (owner_id, name) VALUES ($1, 'Б') RETURNING id::text`, first)
	listC := migrationScalar(t, pool, `INSERT INTO shopping_lists (owner_id, name) VALUES ($1, 'В') RETURNING id::text`, second)
	cases := []struct {
		name, sql, code string
		args            []any
	}{
		{"missing owner", `INSERT INTO shopping_lists (owner_id, name) VALUES (9223372036854775807, 'A')`, "23503", nil},
		{"null owner", `INSERT INTO shopping_lists (name) VALUES ('A')`, "23502", nil},
		{"empty list name", `INSERT INTO shopping_lists (owner_id, name) VALUES ($1, '')`, "23514", []any{first}},
		{"blank list name", `INSERT INTO shopping_lists (owner_id, name) VALUES ($1, U&'\00A0\2003')`, "23514", []any{first}},
		{"long list name", `INSERT INTO shopping_lists (owner_id, name) VALUES ($1, repeat('я',101))`, "23514", []any{first}},
		{"control list name", `INSERT INTO shopping_lists (owner_id, name) VALUES ($1, E'a\nb')`, "23514", []any{first}},
		{"missing list", `INSERT INTO shopping_items (list_id, name) VALUES (9223372036854775807, 'A')`, "23503", nil},
		{"null list", `INSERT INTO shopping_items (name) VALUES ('A')`, "23502", nil},
		{"null name", `INSERT INTO shopping_items (list_id) VALUES ($1)`, "23502", []any{listA}},
		{"empty item name", `INSERT INTO shopping_items (list_id, name) VALUES ($1, '')`, "23514", []any{listA}},
		{"blank item name", `INSERT INTO shopping_items (list_id, name) VALUES ($1, U&'\3000')`, "23514", []any{listA}},
		{"long item name", `INSERT INTO shopping_items (list_id, name) VALUES ($1, repeat('я',201))`, "23514", []any{listA}},
		{"control item name", `INSERT INTO shopping_items (list_id, name) VALUES ($1, U&'a\009Fb')`, "23514", []any{listA}},
		{"long unit", `INSERT INTO shopping_items (list_id, name, unit) VALUES ($1, 'A', repeat('я',33))`, "23514", []any{listA}},
		{"control unit", `INSERT INTO shopping_items (list_id, name, unit) VALUES ($1, 'A', E'\t')`, "23514", []any{listA}},
		{"null unit", `INSERT INTO shopping_items (list_id, name, unit) VALUES ($1, 'A', NULL)`, "23502", []any{listA}},
		{"null mark", `INSERT INTO shopping_items (list_id, name, is_marked) VALUES ($1, 'A', NULL)`, "23502", []any{listA}},
	}
	for _, q := range []string{"0", "-1", "0.0001", "1000000000", "1.2345", "NaN", "Infinity", "-Infinity"} {
		cases = append(cases, struct {
			name, sql, code string
			args            []any
		}{"quantity " + q, `INSERT INTO shopping_items (list_id, name, quantity) VALUES ($1, 'A', $2::numeric)`, "23514", []any{listA, q}})
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := pool.Exec(context.Background(), tc.sql, tc.args...)
			var pgErr *pgconn.PgError
			if !errors.As(err, &pgErr) || pgErr.Code != tc.code {
				t.Fatalf("Expected %s, got %v", tc.code, err)
			}
		})
	}
	migrationExec(t, pool, `INSERT INTO shopping_lists (owner_id, name) VALUES ($1, repeat('я',100))`, first)
	migrationExec(t, pool, `INSERT INTO shopping_items (list_id, name, unit, quantity) VALUES ($1, repeat('я',200), repeat('я',32), 999999999.999), ($1, 'Минимум', '', 0.001), ($1, 'Без количества', '', NULL), ($1, 'Нули', '', 1.0000)`, listA)
	// Буква v не является пробелом (PostgreSQL E'\v' не обозначает vertical tab).
	migrationExec(t, pool, `INSERT INTO shopping_lists (owner_id, name) VALUES ($1, 'v')`, first)
	migrationExec(t, pool, `INSERT INTO shopping_items (list_id, name) VALUES ($1, 'v')`, listA)
	if got := migrationScalar(t, pool, `SELECT quantity::text FROM shopping_items WHERE name = 'Минимум'`); got != "0.001" {
		t.Fatalf("Minimum quantity changed: %s", got)
	}
	if got := migrationScalar(t, pool, `SELECT quantity::text FROM shopping_items WHERE quantity = 999999999.999`); got != "999999999.999" {
		t.Fatalf("Maximum quantity changed: %s", got)
	}
	if _, err := pool.Exec(context.Background(), `UPDATE shopping_items SET quantity = 1.0001 WHERE list_id = $1`, listA); err == nil {
		t.Fatal("UPDATE bypassed quantity constraint")
	}
	migrationExec(t, pool, `INSERT INTO shopping_items (list_id, name) VALUES ($1, 'Б'), ($2, 'В')`, listB, listC)
	migrationExec(t, pool, `DELETE FROM shopping_lists WHERE id = $1`, listA)
	if got := migrationScalar(t, pool, `SELECT count(*)::text FROM shopping_items`); got != "2" {
		t.Fatal("List deletion affected other lists or failed to cascade")
	}
	migrationExec(t, pool, `DELETE FROM users WHERE id = $1`, first)
	if got := migrationScalar(t, pool, `SELECT count(*)::text FROM shopping_lists WHERE owner_id = $1`, first); got != "0" {
		t.Fatal("Owner cascade left lists")
	}
	if got := migrationScalar(t, pool, `SELECT name FROM shopping_items`); got != "В" {
		t.Fatal("Owner cascade affected another user")
	}
	migrationExec(t, pool, `DELETE FROM shopping_lists WHERE id = $1`, listC)
	if got := migrationScalar(t, pool, `SELECT count(*)::text FROM shopping_lists WHERE owner_id = $1`, second); got != "0" {
		t.Fatal("Cannot delete final list")
	}
}
