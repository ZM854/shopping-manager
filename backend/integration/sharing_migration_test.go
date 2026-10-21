//go:build integration

package integration_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/ZM854/shopping-manager/backend/internal/testutil/testdb"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

func sharingMigrationSQL(t *testing.T, direction string) string {
	t.Helper()
	data, err := os.ReadFile("../migrations/000009_shopping_list_shares." + direction + ".sql")
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func sharingMigrationReject(t *testing.T, pool *pgxpool.Pool, code, query string, args ...any) {
	t.Helper()
	_, err := pool.Exec(context.Background(), query, args...)
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != code {
		t.Fatalf("Ожидался SQLSTATE %s, получена ошибка: %v", code, err)
	}
}

func sharingMigrationShare(t *testing.T, pool *pgxpool.Pool, creator, source int64, hashByte byte) int64 {
	t.Helper()
	var id int64
	err := pool.QueryRow(context.Background(), `
		INSERT INTO shopping_list_shares (creator_id, source_list_id, token_hash, name, preserve_marks)
		VALUES ($1, $2, $3, 'Снимок', false) RETURNING id`,
		creator, source, bytes.Repeat([]byte{hashByte}, 32),
	).Scan(&id)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestSharingMigrationFreshDatabase(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	// Как у CLI мигратора: каждый up-файл фиксируется отдельно.
	for version := 0; version < 9; version++ {
		testdb.ApplyMigrations(t, pool, version, version+1)
	}
	if got := migrationScalar(t, pool, `
		SELECT count(*)::text FROM information_schema.tables
		WHERE table_schema = current_schema() AND table_type = 'BASE TABLE'`); got != "7" {
		t.Fatalf("После чистого запуска ожидалось 7 таблиц, получено %s", got)
	}
	for _, table := range []string{"shopping_list_shares", "shopping_list_share_items"} {
		if migrationScalar(t, pool, "SELECT count(*)::text FROM "+table) != "0" {
			t.Fatal("В чистой БД появились снимки или позиции")
		}
	}
	if got := migrationScalar(t, pool, `
		SELECT data_type || ':' || is_nullable || ':' || coalesce(numeric_scale::text, 'unrestricted')
		FROM information_schema.columns
		WHERE table_schema = current_schema()
		  AND table_name = 'shopping_list_share_items' AND column_name = 'quantity'`); got != "numeric:YES:unrestricted" {
		t.Fatalf("Неверная схема количества: %s", got)
	}
	owner := migrationUser(t, pool, "sharing-fresh@example.test")
	if migrationScalar(t, pool, `SELECT count(*)::text FROM shopping_lists`) != "0" {
		t.Fatal("Новому пользователю создан список без его действия")
	}
	share := sharingMigrationShare(t, pool, owner, 9223372036854775807, 1)
	if migrationScalar(t, pool, `
		SELECT (created_at IS NOT NULL AND octet_length(token_hash) = 32)::text
		FROM shopping_list_shares WHERE id = $1`, share) != "true" {
		t.Fatal("Не сохранены время создания или хеш снимка")
	}
	if got := migrationScalar(t, pool, `
		INSERT INTO shopping_list_share_items (share_id, position, name)
		VALUES ($1, 1, 'Молоко')
		RETURNING (quantity IS NULL AND unit = '' AND NOT is_marked)::text`, share); got != "true" {
		t.Fatal("Неверные значения по умолчанию у позиции снимка")
	}
}

const sharingExistingData = `SELECT jsonb_build_object(
	'users', (SELECT jsonb_agg(to_jsonb(u) ORDER BY id) FROM users u),
	'sessions', (SELECT jsonb_agg(to_jsonb(s) ORDER BY id) FROM refresh_tokens s),
	'lists', (SELECT jsonb_agg(to_jsonb(l) ORDER BY id) FROM shopping_lists l),
	'items', (SELECT jsonb_agg(to_jsonb(i) ORDER BY id) FROM shopping_items i),
	'replays', (SELECT jsonb_agg(to_jsonb(r) ORDER BY owner_id, operation, key) FROM shopping_idempotency r),
	'user_sequence', (SELECT jsonb_build_array(last_value, is_called) FROM users_id_seq),
	'session_sequence', (SELECT jsonb_build_array(last_value, is_called) FROM refresh_tokens_id_seq),
	'list_sequence', (SELECT jsonb_build_array(last_value, is_called) FROM shopping_lists_id_seq),
	'item_sequence', (SELECT jsonb_build_array(last_value, is_called) FROM shopping_items_id_seq)
)::text`

func TestSharingMigrationPreservesVersion8DataAndPOSTReplay(t *testing.T) {
	t.Parallel()
	f := newShoppingFixture(t)
	list := f.list(t, f.first, "sharing-upgrade-list-0001")
	item := f.item(t, f.first, list.ID, "sharing-upgrade-item-0001")
	otherList := f.list(t, f.second, "sharing-upgrade-list-0002")
	for version := 5; version < 8; version++ {
		testdb.ApplyMigrations(t, f.pool, version, version+1)
	}
	migrationExec(t, f.pool, `UPDATE users SET is_email_verified = true WHERE id = $1`, f.first)
	migrationExec(t, f.pool, `
		UPDATE users SET activation_token_hash = repeat('c', 64),
			activation_expires_at = clock_timestamp() + interval '1 day',
			activation_next_send_at = clock_timestamp() + interval '1 minute'
		WHERE id = $1`, f.second)
	migrationExec(t, f.pool, `
		INSERT INTO refresh_tokens (session_id, user_id, token_hash, expires_at)
		VALUES ('12345678-1234-4123-8123-123456789abc', $1, repeat('a', 64), clock_timestamp() + interval '30 days'),
		       ('12345678-1234-4123-8123-123456789abd', $1, repeat('b', 64), clock_timestamp() + interval '1 day')`, f.first)
	migrationExec(t, f.pool, `
		INSERT INTO shopping_items (list_id, name, quantity, unit, is_marked)
		VALUES ($1, 'Без количества', NULL, '', false),
		       ($1, 'Минимум', 0.001, 'кг', true),
		       ($1, 'Максимум', 999999999.999, 'шт', false)`, otherList.ID)
	migrationExec(t, f.pool, `
		SELECT setval('users_id_seq', 9007199254741000),
		       setval('refresh_tokens_id_seq', 9007199254741000),
		       setval('shopping_lists_id_seq', 9007199254741000),
		       setval('shopping_items_id_seq', 9007199254741000)`)
	before := migrationScalar(t, f.pool, sharingExistingData)
	testdb.ApplyMigrations(t, f.pool, 8, 9)
	if migrationScalar(t, f.pool, sharingExistingData) != before {
		t.Fatal("Обновление изменило аккаунты, активацию, сессии, покупки, повторы POST или последовательности")
	}
	if !reflect.DeepEqual(f.list(t, f.first, "sharing-upgrade-list-0001"), list) ||
		!reflect.DeepEqual(f.item(t, f.first, list.ID, "sharing-upgrade-item-0001"), item) {
		t.Fatal("После обновления изменился исходный ответ повторного POST")
	}
	if migrationScalar(t, f.pool, sharingExistingData) != before {
		t.Fatal("Повтор прежнего POST создал дубль или изменил сохранённые данные")
	}
	if got := migrationScalar(t, f.pool, `
		INSERT INTO shopping_lists (owner_id, name) VALUES ($1, 'Новый') RETURNING id::text`, f.first); got != "9007199254741001" {
		t.Fatalf("После обновления сбилась последовательность списков: %s", got)
	}
}

func TestSharingMigrationConstraintsAndExactQuantities(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	testdb.ApplyMigrations(t, pool, 0, 9)
	owner := migrationUser(t, pool, "sharing-constraints@example.test")
	share := sharingMigrationShare(t, pool, owner, 1, 1)
	// Несколько снимков одного источника допустимы.
	sharingMigrationShare(t, pool, owner, 1, 2)
	migrationExec(t, pool, `
		INSERT INTO shopping_list_shares (creator_id, source_list_id, token_hash, name, preserve_marks)
		VALUES ($1, 1, decode(repeat('03', 32), 'hex'), repeat('🥛', 100), true)`, owner)

	sharingMigrationReject(t, pool, "23505", `
		INSERT INTO shopping_list_shares (creator_id, source_list_id, token_hash, name, preserve_marks)
		VALUES ($1, 1, $2, 'Снимок', false)`, owner, bytes.Repeat([]byte{1}, 32))
	sharingMigrationReject(t, pool, "23502", `
		INSERT INTO shopping_list_shares (creator_id, source_list_id, token_hash, name)
		VALUES ($1, 1, $2, 'Снимок')`, owner, make([]byte, 32))
	sharingMigrationReject(t, pool, "23503", `
		INSERT INTO shopping_list_share_items (share_id, position, name)
		VALUES (9223372036854775807, 1, 'Товар')`)

	shareCases := []struct {
		name       string
		assignment string
		code       string
	}{
		{"missing creator", "creator_id = 9223372036854775807", "23503"},
		{"zero source", "source_list_id = 0", "23514"},
		{"negative source", "source_list_id = -1", "23514"},
		{"zero id", "id = 0", "23514"},
		{"short hash", "token_hash = decode(repeat('00', 31), 'hex')", "23514"},
		{"long hash", "token_hash = decode(repeat('00', 33), 'hex')", "23514"},
		{"null hash", "token_hash = NULL", "23502"},
		{"empty list name", "name = ''", "23514"},
		{"blank list name", `name = U&'\00A0\2003\3000'`, "23514"},
		{"long list name", "name = repeat('🥛', 101)", "23514"},
		{"control list name", `name = E'a\nb'`, "23514"},
		{"C1 list name", `name = U&'a\0085b'`, "23514"},
	}
	for _, tc := range shareCases {
		t.Run(tc.name, func(t *testing.T) {
			// Фрагмент SQL выбирается только из фиксированной таблицы теста.
			sharingMigrationReject(t, pool, tc.code,
				"UPDATE shopping_list_shares SET "+tc.assignment+" WHERE id = $1", share)
		})
	}
	itemCases := []struct {
		name     string
		position int64
		itemName string
		unit     string
	}{
		{"zero position", 0, "Товар", ""},
		{"negative position", -1, "Товар", ""},
		{"empty item name", 1, "", ""},
		{"blank item name", 1, "\u00a0\u3000", ""},
		{"long item name", 1, strings.Repeat("🥛", 201), ""},
		{"control item name", 1, "a\u009fb", ""},
		{"long unit", 1, "Товар", strings.Repeat("я", 33)},
		{"control unit", 1, "Товар", "\t"},
	}
	for _, tc := range itemCases {
		t.Run(tc.name, func(t *testing.T) {
			sharingMigrationReject(t, pool, "23514", `
				INSERT INTO shopping_list_share_items (share_id, position, name, unit)
				VALUES ($1, $2, $3, $4)`, share, tc.position, tc.itemName, tc.unit)
		})
	}
	for _, quantity := range []string{
		"0", "-1", "0.0001", "1000000000", "1.0001", "NaN", "Infinity", "-Infinity",
	} {
		t.Run("quantity "+quantity, func(t *testing.T) {
			sharingMigrationReject(t, pool, "23514", `
				INSERT INTO shopping_list_share_items (share_id, position, name, quantity)
				VALUES ($1, 1, 'Товар', $2::numeric)`, share, quantity)
		})
	}
	migrationExec(t, pool, `
		INSERT INTO shopping_list_share_items (share_id, position, name, quantity, unit, is_marked)
		VALUES ($1, 4, 'Товар', 999999999.999, '', false),
		       ($1, 2, 'Товар', 0.001, 'кг', true),
		       ($1, 3, 'Товар', 2.675, 'л', false),
		       ($1, 1, repeat('🥛', 200), NULL, repeat('я', 32), true)`, share)
	if got := migrationScalar(t, pool, `
		SELECT jsonb_agg(quantity::text ORDER BY position)::text
		FROM shopping_list_share_items WHERE share_id = $1`, share); got != `[null, "0.001", "2.675", "999999999.999"]` {
		t.Fatalf("Изменились точность, null или порядок количеств: %s", got)
	}
	sharingMigrationReject(t, pool, "23505", `
		INSERT INTO shopping_list_share_items (share_id, position, name)
		VALUES ($1, 1, 'Другая позиция')`, share)
	sharingMigrationReject(t, pool, "23514", `
		UPDATE shopping_list_share_items SET quantity = 2.6751 WHERE share_id = $1`, share)
}

func TestSharingMigrationIdempotencyOperations(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	testdb.ApplyMigrations(t, pool, 0, 9)
	owner := migrationUser(t, pool, "sharing-operations@example.test")
	insert := `INSERT INTO shopping_idempotency
		(owner_id, operation, key, request_hash, resource_id, response, expires_at)
		VALUES ($1, $2, 'sharing-operation-key-0001', repeat('a', 64),
		        9223372036854775807, '{"id":"9223372036854775807"}', clock_timestamp() + interval '1 day')`
	for _, operation := range []string{"lists", "items:1", "shares:1", "shares:9223372036854775807", "share-imports"} {
		// Результат может ссылаться на уже удалённый ресурс, поэтому FK к нему нет.
		migrationExec(t, pool, insert, owner, operation)
	}
	for _, operation := range []string{
		"", "products", "items:0", "shares:0", "shares:-1", "shares:01", "shares:1x",
		"shares:12345678901234567890", "share-imports:1", "share-imports ",
	} {
		t.Run(operation, func(t *testing.T) {
			sharingMigrationReject(t, pool, "23514", insert, owner, operation)
		})
	}
	if migrationScalar(t, pool, `SELECT count(*)::text FROM shopping_idempotency`) != "5" {
		t.Fatal("Новые scope пересекаются или ограничения идемпотентности потеряны")
	}
}

func TestSharingMigrationSourceDeletionAndOwnerCascade(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	testdb.ApplyMigrations(t, pool, 0, 9)
	author := migrationUser(t, pool, "sharing-author@example.test")
	recipient := migrationUser(t, pool, "sharing-recipient@example.test")
	var source int64
	if err := pool.QueryRow(context.Background(), `
		INSERT INTO shopping_lists (owner_id, name) VALUES ($1, 'Исходник') RETURNING id`, author).Scan(&source); err != nil {
		t.Fatal(err)
	}
	share := sharingMigrationShare(t, pool, author, source, 1)
	migrationExec(t, pool, `
		INSERT INTO shopping_list_share_items (share_id, position, name, quantity, is_marked)
		VALUES ($1, 1, 'Молоко', 2.675, true)`, share)
	// Здесь проверяются связи схемы; API импорта появится на следующих этапах.
	for range 2 {
		list := migrationScalar(t, pool, `
			INSERT INTO shopping_lists (owner_id, name)
			SELECT $1, name FROM shopping_list_shares WHERE id = $2 RETURNING id::text`, recipient, share)
		migrationExec(t, pool, `
			INSERT INTO shopping_items (list_id, name, quantity, unit, is_marked)
			SELECT $1, name, quantity, unit, is_marked
			FROM shopping_list_share_items WHERE share_id = $2 ORDER BY position`, list, share)
	}
	snapshot := `SELECT jsonb_build_object(
		'lists', (SELECT jsonb_agg(to_jsonb(l) ORDER BY id) FROM shopping_lists l WHERE owner_id = $1),
		'items', (SELECT jsonb_agg(to_jsonb(i) ORDER BY i.id)
			FROM shopping_items i JOIN shopping_lists l ON l.id = i.list_id WHERE l.owner_id = $1)
	)::text`
	before := migrationScalar(t, pool, snapshot, recipient)
	migrationExec(t, pool, `DELETE FROM shopping_lists WHERE id = $1`, source)
	if migrationScalar(t, pool, `SELECT count(*)::text FROM shopping_list_shares`) != "1" ||
		migrationScalar(t, pool, `SELECT count(*)::text FROM shopping_list_share_items`) != "1" {
		t.Fatal("Удаление оригинала удалило снимок или его позиции")
	}
	migrationExec(t, pool, `DELETE FROM users WHERE id = $1`, author)
	if migrationScalar(t, pool, `SELECT count(*)::text FROM shopping_list_shares`) != "0" ||
		migrationScalar(t, pool, `SELECT count(*)::text FROM shopping_list_share_items`) != "0" {
		t.Fatal("После удаления автора остались его снимки или позиции")
	}
	if migrationScalar(t, pool, snapshot, recipient) != before ||
		migrationScalar(t, pool, `SELECT count(*)::text FROM shopping_lists WHERE owner_id = $1`, recipient) != "2" {
		t.Fatal("Каскад затронул независимые копии получателя")
	}
}

func TestSharingMigrationFailureRollsBackSchemaAndOperationCheck(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	testdb.ApplyMigrations(t, pool, 0, 8)
	owner := migrationUser(t, pool, "sharing-failure@example.test")
	migrationExec(t, pool, `INSERT INTO shopping_lists (owner_id, name) VALUES ($1, 'Сохраняемый список')`, owner)
	before := migrationScalar(t, pool, sharingExistingData)
	constraintSQL := `SELECT pg_get_constraintdef(oid) FROM pg_constraint
		WHERE conrelid = 'shopping_idempotency'::regclass AND conname = 'shopping_idempotency_operation_check'`
	beforeConstraint := migrationScalar(t, pool, constraintSQL)
	// Конфликт возникает после расширения CHECK и создания заголовка снимка.
	migrationExec(t, pool, `CREATE TABLE shopping_list_share_items (sentinel text NOT NULL)`)
	migrationExec(t, pool, `INSERT INTO shopping_list_share_items VALUES ('Сохранить')`)
	sharingMigrationReject(t, pool, "42P07", sharingMigrationSQL(t, "up"))
	if migrationScalar(t, pool, sharingExistingData) != before ||
		migrationScalar(t, pool, constraintSQL) != beforeConstraint {
		t.Fatal("Неудачная миграция изменила данные или CHECK операций")
	}
	for _, relation := range []string{
		"shopping_list_shares", "shopping_list_shares_id_seq", "shopping_list_shares_creator_id_id_idx",
	} {
		if migrationScalar(t, pool, `SELECT (to_regclass($1) IS NULL)::text`, relation) != "true" {
			t.Fatalf("Неудачная миграция оставила объект %s", relation)
		}
	}
	if migrationScalar(t, pool, `SELECT sentinel FROM shopping_list_share_items`) != "Сохранить" {
		t.Fatal("Неудачная миграция затронула существовавшую таблицу")
	}
	// Удаляется только созданная этим тестом помеха в его изолированной схеме.
	migrationExec(t, pool, `DROP TABLE shopping_list_share_items`)
	migrationExec(t, pool, sharingMigrationSQL(t, "up"))
	if migrationScalar(t, pool, sharingExistingData) != before {
		t.Fatal("Повтор после устранения причины сбоя изменил существовавшие данные")
	}
}

func TestSharingMigrationDownRefusesWithoutDataLoss(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	testdb.ApplyMigrations(t, pool, 0, 9)
	owner := migrationUser(t, pool, "sharing-down@example.test")
	share := sharingMigrationShare(t, pool, owner, 1, 1)
	migrationExec(t, pool, `
		INSERT INTO shopping_list_share_items (share_id, position, name) VALUES ($1, 1, 'Товар')`, share)
	snapshot := `SELECT jsonb_build_object(
		'shares', (SELECT jsonb_agg(to_jsonb(s) ORDER BY id) FROM shopping_list_shares s),
		'items', (SELECT jsonb_agg(to_jsonb(i) ORDER BY share_id, position) FROM shopping_list_share_items i)
	)::text`
	before := migrationScalar(t, pool, snapshot)
	sharingMigrationReject(t, pool, "0A000", sharingMigrationSQL(t, "down"))
	if migrationScalar(t, pool, snapshot) != before {
		t.Fatal("Отказ down изменил снимки или позиции")
	}
	if !strings.Contains(migrationScalar(t, pool, `
		SELECT pg_get_constraintdef(oid) FROM pg_constraint
		WHERE conrelid = 'shopping_idempotency'::regclass
		  AND conname = 'shopping_idempotency_operation_check'`), "share-imports") {
		t.Fatal("Отказ down изменил допустимые операции идемпотентности")
	}
}
