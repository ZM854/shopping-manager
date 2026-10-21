//go:build integration

package integration_test

import (
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

func recipeMigrationSQL(t *testing.T, direction string) string {
	t.Helper()
	data, err := os.ReadFile("../migrations/000010_recipes." + direction + ".sql")
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func recipeMigrationReject(t *testing.T, pool *pgxpool.Pool, code, query string, args ...any) {
	t.Helper()
	_, err := pool.Exec(context.Background(), query, args...)
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != code {
		t.Fatalf("Ожидался SQLSTATE %s, получена ошибка: %v", code, err)
	}
}

func recipeMigrationCreate(t *testing.T, pool *pgxpool.Pool, owner int64) string {
	t.Helper()
	return migrationScalar(t, pool, `
		INSERT INTO recipes (owner_id, title, cooking_time_minutes, category_code)
		VALUES ($1, 'Омлет', 15, 'main-dishes') RETURNING id::text`, owner)
}

func recipeMigrationFixture(t *testing.T) (*pgxpool.Pool, int64, string) {
	t.Helper()
	pool := testdb.New(t)
	testdb.ApplyMigrations(t, pool, 0, 10)
	owner := migrationUser(t, pool, "recipe@example.test")
	return pool, owner, recipeMigrationCreate(t, pool, owner)
}

func TestRecipeMigrationFreshDatabase(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	// Каждый up фиксируется отдельно, как при запуске CLI мигратора.
	for version := 0; version < 10; version++ {
		testdb.ApplyMigrations(t, pool, version, version+1)
	}
	if migrationScalar(t, pool, `
		SELECT count(*)::text FROM information_schema.tables
		WHERE table_schema = current_schema() AND table_type = 'BASE TABLE'`) != "11" {
		t.Fatal("После чистого запуска ожидалось 11 таблиц")
	}
	for _, table := range []string{"recipes", "recipe_ingredients", "recipe_steps", "recipe_idempotency"} {
		if migrationScalar(t, pool, "SELECT count(*)::text FROM "+table) != "0" {
			t.Fatalf("В чистой БД таблица %s не пуста", table)
		}
	}
	owner := migrationUser(t, pool, "recipe-fresh@example.test")
	if migrationScalar(t, pool, `SELECT count(*)::text FROM recipes WHERE owner_id = $1`, owner) != "0" {
		t.Fatal("Новому пользователю создан рецепт без его действия")
	}
	recipe := recipeMigrationCreate(t, pool, owner)
	if recipe != "1" || migrationScalar(t, pool, `
		SELECT (created_at IS NOT NULL AND updated_at = created_at)::text
		FROM recipes WHERE id = $1`, recipe) != "true" {
		t.Fatal("Неверная последовательность рецептов или временные поля по умолчанию")
	}
	if migrationScalar(t, pool, `
		INSERT INTO recipe_ingredients (recipe_id, position, name)
		VALUES ($1, 1, 'Соль') RETURNING (quantity IS NULL AND unit = '')::text`, recipe) != "true" {
		t.Fatal("Неверные значения по умолчанию у ингредиента")
	}
	if got := migrationScalar(t, pool, `
		SELECT data_type || ':' || is_nullable || ':' || coalesce(numeric_scale::text, 'unrestricted')
		FROM information_schema.columns
		WHERE table_schema = current_schema()
		  AND table_name = 'recipe_ingredients' AND column_name = 'quantity'`); got != "numeric:YES:unrestricted" {
		t.Fatalf("Неверная схема количества: %s", got)
	}
	for _, index := range []struct{ name, columns string }{
		{"recipes_owner_id_id_idx", "(owner_id, id)"},
		{"recipe_ingredients_pkey", "(recipe_id, position)"},
		{"recipe_steps_pkey", "(recipe_id, position)"},
		{"recipe_idempotency_pkey", "(owner_id, key)"},
		{"recipe_idempotency_expires_at_idx", "(expires_at)"},
	} {
		definition := migrationScalar(t, pool, `
			SELECT indexdef FROM pg_indexes WHERE schemaname = current_schema() AND indexname = $1`, index.name)
		// PostgreSQL заключает имя position в кавычки при выводе определения.
		if !strings.Contains(strings.ReplaceAll(definition, `"`, ""), index.columns) {
			t.Fatalf("Неверные колонки индекса %s", index.name)
		}
	}
}

// Сравниваются все старые строки и состояния последовательностей без вывода их содержимого.
const recipePreviousData = `SELECT jsonb_build_object(
	'base', (` + sharingExistingData + `)::jsonb,
	'shares', (SELECT jsonb_agg(to_jsonb(s) ORDER BY id) FROM shopping_list_shares s),
	'share_items', (SELECT jsonb_agg(to_jsonb(i) ORDER BY share_id, position) FROM shopping_list_share_items i),
	'share_sequence', (SELECT jsonb_build_array(last_value, is_called) FROM shopping_list_shares_id_seq)
)::text`

func TestRecipeMigrationPreservesVersion9DataAndPOSTReplays(t *testing.T) {
	t.Parallel()
	f := newSharingFixture(t)
	list := f.list(t)
	items := f.items(t, list)
	share := f.share(t, list, 1, true)
	copy := f.importList(t, f.recipient, 1, "recipe-upgrade-import-0001")
	migrationExec(t, f.pool, `UPDATE users SET is_email_verified = true WHERE id = $1`, f.author)
	migrationExec(t, f.pool, `
		UPDATE users SET activation_token_hash = repeat('c', 64),
			activation_expires_at = clock_timestamp() + interval '1 day',
			activation_next_send_at = clock_timestamp() + interval '1 minute'
		WHERE id = $1`, f.recipient)
	migrationExec(t, f.pool, `
		INSERT INTO refresh_tokens (session_id, user_id, token_hash, expires_at)
		VALUES ('12345678-1234-4123-8123-123456789abc', $1, repeat('a', 64), clock_timestamp() + interval '30 days'),
		       ('12345678-1234-4123-8123-123456789abd', $1, repeat('b', 64), clock_timestamp() + interval '1 day')`, f.author)
	migrationExec(t, f.pool, `
		SELECT setval('users_id_seq', 9007199254741000),
		       setval('refresh_tokens_id_seq', 9007199254741000),
		       setval('shopping_lists_id_seq', 9007199254741000),
		       setval('shopping_items_id_seq', 9007199254741000),
		       setval('shopping_list_shares_id_seq', 9007199254741000)`)
	before := migrationScalar(t, f.pool, recipePreviousData)
	testdb.ApplyMigrations(t, f.pool, 9, 10)
	if migrationScalar(t, f.pool, recipePreviousData) != before {
		t.Fatal("Миграция изменила аккаунты, активацию, сессии, покупки, снимки, POST replay или последовательности")
	}
	if !reflect.DeepEqual(f.list(t), list) || !reflect.DeepEqual(f.items(t, list), items) ||
		!reflect.DeepEqual(f.share(t, list, 1, true), share) ||
		!reflect.DeepEqual(f.importList(t, f.recipient, 1, "recipe-upgrade-import-0001"), copy) {
		t.Fatal("После обновления изменились исходные ответы POST")
	}
	if migrationScalar(t, f.pool, recipePreviousData) != before {
		t.Fatal("Повтор прежних POST после обновления изменил сохранённые данные")
	}
	if recipeMigrationCreate(t, f.pool, f.author) != "1" {
		t.Fatal("У новых рецептов нет независимой последовательности ID")
	}
	if migrationScalar(t, f.pool, `
		INSERT INTO shopping_lists (owner_id, name) VALUES ($1, 'Новый') RETURNING id::text`, f.author) != "9007199254741001" {
		t.Fatal("После обновления сбилась последовательность списков")
	}
}

func TestRecipeMigrationRecipeConstraints(t *testing.T) {
	t.Parallel()
	pool, owner, recipe := recipeMigrationFixture(t)
	// Одинаковые названия у одного владельца разрешены.
	recipeMigrationCreate(t, pool, owner)
	for _, category := range []string{
		"soups", "salads", "starters", "main-dishes", "side-dishes",
		"baking", "desserts", "sauces", "drinks", "other",
	} {
		migrationExec(t, pool, `UPDATE recipes SET category_code = $2 WHERE id = $1`, recipe, category)
	}
	migrationExec(t, pool, `UPDATE recipes SET title = repeat('🍳', 200), cooking_time_minutes = 1440 WHERE id = $1`, recipe)
	migrationExec(t, pool, `UPDATE recipes SET cooking_time_minutes = 5 WHERE id = $1`, recipe)
	for _, tc := range []struct {
		name, assignment, code string
	}{
		{"zero id", "id = 0", "23514"},
		{"negative id", "id = -1", "23514"},
		{"missing owner", "owner_id = 9223372036854775807", "23503"},
		{"null owner", "owner_id = NULL", "23502"},
		{"empty title", "title = ''", "23514"},
		{"blank title", `title = U&'\00A0\2003\3000'`, "23514"},
		{"long title", "title = repeat('🍳', 201)", "23514"},
		{"control title", `title = E'a\nb'`, "23514"},
		{"C1 title", `title = U&'a\0085b'`, "23514"},
		{"null title", "title = NULL", "23502"},
		{"short time", "cooking_time_minutes = 0", "23514"},
		{"negative time", "cooking_time_minutes = -5", "23514"},
		{"long time", "cooking_time_minutes = 1445", "23514"},
		{"time step", "cooking_time_minutes = 7", "23514"},
		{"null time", "cooking_time_minutes = NULL", "23502"},
		{"unknown category", "category_code = 'breakfast'", "23514"},
		{"empty category", "category_code = ''", "23514"},
		{"noncanonical category", "category_code = 'Soups'", "23514"},
		{"padded category", "category_code = 'soups '", "23514"},
		{"null category", "category_code = NULL", "23502"},
		{"null created", "created_at = NULL", "23502"},
		{"null updated", "updated_at = NULL", "23502"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Фрагмент SQL выбирается только из фиксированной таблицы теста.
			recipeMigrationReject(t, pool, tc.code, "UPDATE recipes SET "+tc.assignment+" WHERE id = $1", recipe)
		})
	}
}

func TestRecipeMigrationIngredientConstraintsAndExactQuantities(t *testing.T) {
	t.Parallel()
	pool, _, recipe := recipeMigrationFixture(t)
	for _, tc := range []struct {
		name, ingredient, unit string
	}{
		{"empty name", "", ""},
		{"blank name", "\u00a0\u3000", ""},
		{"long name", strings.Repeat("🍳", 201), ""},
		{"control name", "a\nb", ""},
		{"C1 name", "a\u009fb", ""},
		{"long unit", "Яйца", strings.Repeat("я", 33)},
		{"control unit", "Яйца", "\t"},
		{"C1 unit", "Яйца", "\u0085"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			recipeMigrationReject(t, pool, "23514", `
				INSERT INTO recipe_ingredients (recipe_id, position, name, unit)
				VALUES ($1, 1, $2, $3)`, recipe, tc.ingredient, tc.unit)
		})
	}
	for _, quantity := range []string{
		"0", "-1", "0.0001", "1000000000", "999999999.9991", "1.0001", "NaN", "Infinity", "-Infinity",
	} {
		t.Run("quantity "+quantity, func(t *testing.T) {
			recipeMigrationReject(t, pool, "23514", `
				INSERT INTO recipe_ingredients (recipe_id, position, name, quantity)
				VALUES ($1, 1, 'Яйца', $2::numeric)`, recipe, quantity)
		})
	}
	// Количество и единица независимы, повторяющиеся названия сохраняются.
	migrationExec(t, pool, `
		INSERT INTO recipe_ingredients (recipe_id, position, name, quantity, unit)
		VALUES ($1, 4, 'Яйца', 999999999.999, ''),
		       ($1, 2, 'Яйца', 0.001, 'кг'),
		       ($1, 3, 'Яйца', 2.675, ''),
		       ($1, 1, repeat('🍳', 200), NULL, repeat('я', 32))`, recipe)
	if migrationScalar(t, pool, `
		SELECT jsonb_agg(quantity::text ORDER BY position)::text
		FROM recipe_ingredients WHERE recipe_id = $1`, recipe) != `[null, "0.001", "2.675", "999999999.999"]` {
		t.Fatal("Изменились точность, null или порядок количеств")
	}
	recipeMigrationReject(t, pool, "23514", `
		UPDATE recipe_ingredients SET quantity = 2.6751 WHERE recipe_id = $1`, recipe)
	for _, table := range []string{"recipe_ingredients", "recipe_steps"} {
		column, value := "name", "Яйца"
		if table == "recipe_steps" {
			column, value = "text", "Приготовить"
		}
		insert := "INSERT INTO " + table + " (recipe_id, position, " + column + ") VALUES ($1, $2, $3)"
		recipeMigrationReject(t, pool, "23503", insert, int64(9223372036854775807), 1, value)
		recipeMigrationReject(t, pool, "23502", insert, nil, 1, value)
		recipeMigrationReject(t, pool, "23514", insert, recipe, 0, value)
		recipeMigrationReject(t, pool, "23514", insert, recipe, -1, value)
		recipeMigrationReject(t, pool, "23502", insert, recipe, nil, value)
		recipeMigrationReject(t, pool, "23502", insert, recipe, 5, nil)
		if table == "recipe_steps" {
			migrationExec(t, pool, insert, recipe, 1, value)
		}
		recipeMigrationReject(t, pool, "23505", insert, recipe, 1, value)
	}
	recipeMigrationReject(t, pool, "23502", `
		UPDATE recipe_ingredients SET unit = NULL WHERE recipe_id = $1`, recipe)
}

func TestRecipeMigrationStepTextConstraintsAndOrder(t *testing.T) {
	t.Parallel()
	pool, _, recipe := recipeMigrationFixture(t)
	for _, text := range []string{
		"", " \t\n\u00a0\u2003\u3000", strings.Repeat("🍳", 20001),
		"a\u0001b", "a\u0008b", "a\vb", "a\fb", "a\rb", "a\u001fb", "a\u007fb", "a\u0085b", "a\u009fb",
	} {
		recipeMigrationReject(t, pool, "23514", `
			INSERT INTO recipe_steps (recipe_id, position, text) VALUES ($1, 1, $2)`, recipe, text)
	}
	longStep := strings.Repeat("🍳", 20000)
	multiline := "Взбить яйца.\n\tДобавить соль и приготовить."
	migrationExec(t, pool, `
		INSERT INTO recipe_steps (recipe_id, position, text)
		VALUES ($1, 2, $2), ($1, 1, $3)`, recipe, multiline, longStep)
	if migrationScalar(t, pool, `SELECT text FROM recipe_steps WHERE recipe_id = $1 AND position = 2`, recipe) != multiline {
		t.Fatal("Изменились переносы строк или отступы текста шага")
	}
	if migrationScalar(t, pool, `
		SELECT (array_agg(text ORDER BY position) = ARRAY[$2, $3]::text[])::text
		FROM recipe_steps WHERE recipe_id = $1`, recipe, longStep, multiline) != "true" {
		t.Fatal("Изменился порядок или текст шагов")
	}
}

func TestRecipeMigrationIdempotencyConstraints(t *testing.T) {
	t.Parallel()
	pool, owner, recipe := recipeMigrationFixture(t)
	other := migrationUser(t, pool, "recipe-other@example.test")
	insert := `INSERT INTO recipe_idempotency (owner_id, key, request_hash, resource_id, response, expires_at)
		VALUES ($1, $2, repeat('a', 64), $3, $4::jsonb, clock_timestamp() + interval '1 day')`
	key := "recipe-create-action-0001"
	response := `{"id":"` + recipe + `"}`
	migrationExec(t, pool, insert, owner, key, recipe, response)
	recipeMigrationReject(t, pool, "23505", insert, owner, key, recipe, response)
	// Один ключ у разных владельцев не конфликтует; FK к результату отсутствует.
	migrationExec(t, pool, insert, other, key, int64(9223372036854775807), `{"id":"9223372036854775807"}`)
	migrationExec(t, pool, insert, owner, strings.Repeat("x", 16), nil, nil)
	migrationExec(t, pool, insert, owner, strings.Repeat("x", 128), nil, nil)
	for _, invalidKey := range []string{
		"", strings.Repeat("x", 15), strings.Repeat("x", 129), "invalid-key space", "неподходящий-ключ",
	} {
		recipeMigrationReject(t, pool, "23514", insert, owner, invalidKey, nil, nil)
	}
	for _, tc := range []struct {
		name, assignment, code string
	}{
		{"missing owner", "owner_id = 9223372036854775807", "23503"},
		{"null owner", "owner_id = NULL", "23502"},
		{"null key", "key = NULL", "23502"},
		{"short hash", "request_hash = repeat('a', 63)", "23514"},
		{"long hash", "request_hash = repeat('a', 65)", "23514"},
		{"uppercase hash", "request_hash = repeat('A', 64)", "23514"},
		{"nonhex hash", "request_hash = repeat('g', 64)", "23514"},
		{"null hash", "request_hash = NULL", "23502"},
		{"zero resource", "resource_id = 0", "23514"},
		{"negative resource", "resource_id = -1", "23514"},
		{"missing resource", "resource_id = NULL", "23514"},
		{"missing response", "response = NULL", "23514"},
		{"json null response", "response = 'null'::jsonb", "23514"},
		{"scalar response", "response = '1'::jsonb", "23514"},
		{"array response", "response = '[]'::jsonb", "23514"},
		{"null expiry", "expires_at = NULL", "23502"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			recipeMigrationReject(t, pool, tc.code,
				"UPDATE recipe_idempotency SET "+tc.assignment+" WHERE owner_id = $1 AND key = $2", owner, key)
		})
	}
	// Истёкшие записи остаются доступными для замены service, а не удаляются БД.
	migrationExec(t, pool, `
		UPDATE recipe_idempotency SET expires_at = clock_timestamp() - interval '1 day'
		WHERE owner_id = $1 AND key = $2`, owner, key)
}

const recipeCurrentData = `SELECT jsonb_build_object(
	'recipes', (SELECT jsonb_agg(to_jsonb(r) ORDER BY id) FROM recipes r),
	'ingredients', (SELECT jsonb_agg(to_jsonb(i) ORDER BY recipe_id, position) FROM recipe_ingredients i),
	'steps', (SELECT jsonb_agg(to_jsonb(s) ORDER BY recipe_id, position) FROM recipe_steps s),
	'replays', (SELECT jsonb_agg(to_jsonb(r) ORDER BY owner_id, key) FROM recipe_idempotency r),
	'sequence', (SELECT jsonb_build_array(last_value, is_called) FROM recipes_id_seq)
)::text`

func TestRecipeMigrationRecipeDeletionAndOwnerCascade(t *testing.T) {
	t.Parallel()
	pool, owner, recipe := recipeMigrationFixture(t)
	other := migrationUser(t, pool, "recipe-cascade-other@example.test")
	otherRecipe := recipeMigrationCreate(t, pool, other)
	secondRecipe := recipeMigrationCreate(t, pool, owner)
	for _, id := range []string{recipe, secondRecipe, otherRecipe} {
		migrationExec(t, pool, `INSERT INTO recipe_ingredients (recipe_id, position, name) VALUES ($1, 1, 'Яйца')`, id)
		migrationExec(t, pool, `INSERT INTO recipe_steps (recipe_id, position, text) VALUES ($1, 1, 'Приготовить')`, id)
	}
	for _, user := range []int64{owner, other} {
		migrationExec(t, pool, `
			INSERT INTO recipe_idempotency (owner_id, key, request_hash, resource_id, response, expires_at)
			VALUES ($1, 'recipe-cascade-action', repeat('a', 64), $2::bigint, jsonb_build_object('id', $2::bigint::text),
			        clock_timestamp() + interval '1 day')`, user, recipe)
	}
	// Чужой replay и рецепт должны пережить удаление другого владельца.
	otherBefore := migrationScalar(t, pool, `
		SELECT jsonb_build_object(
			'recipe', (SELECT to_jsonb(r) FROM recipes r WHERE id = $1),
			'ingredients', (SELECT jsonb_agg(to_jsonb(i)) FROM recipe_ingredients i WHERE recipe_id = $1),
			'steps', (SELECT jsonb_agg(to_jsonb(s)) FROM recipe_steps s WHERE recipe_id = $1),
			'replay', (SELECT to_jsonb(r) FROM recipe_idempotency r WHERE owner_id = $2)
		)::text`, otherRecipe, other)
	migrationExec(t, pool, `DELETE FROM recipes WHERE id = $1`, recipe)
	for _, table := range []string{"recipe_ingredients", "recipe_steps"} {
		if migrationScalar(t, pool, "SELECT count(*)::text FROM "+table+" WHERE recipe_id = $1", recipe) != "0" {
			t.Fatalf("Удаление рецепта оставило строки %s", table)
		}
	}
	if migrationScalar(t, pool, `SELECT count(*)::text FROM recipe_idempotency`) != "2" {
		t.Fatal("Удаление рецепта уничтожило replay")
	}
	migrationExec(t, pool, `DELETE FROM users WHERE id = $1`, owner)
	for _, table := range []string{"recipes", "recipe_ingredients", "recipe_steps", "recipe_idempotency"} {
		if migrationScalar(t, pool, "SELECT count(*)::text FROM "+table) != "1" {
			t.Fatalf("Каскад владельца оставил его данные или затронул чужие в %s", table)
		}
	}
	otherAfter := migrationScalar(t, pool, `
		SELECT jsonb_build_object(
			'recipe', (SELECT to_jsonb(r) FROM recipes r WHERE id = $1),
			'ingredients', (SELECT jsonb_agg(to_jsonb(i)) FROM recipe_ingredients i WHERE recipe_id = $1),
			'steps', (SELECT jsonb_agg(to_jsonb(s)) FROM recipe_steps s WHERE recipe_id = $1),
			'replay', (SELECT to_jsonb(r) FROM recipe_idempotency r WHERE owner_id = $2)
		)::text`, otherRecipe, other)
	if otherAfter != otherBefore {
		t.Fatal("Удаление владельца изменило данные другого пользователя")
	}
}

func TestRecipeMigrationFailureRollsBackAllNewObjects(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	testdb.ApplyMigrations(t, pool, 0, 9)
	owner := migrationUser(t, pool, "recipe-failure@example.test")
	migrationExec(t, pool, `INSERT INTO shopping_lists (owner_id, name) VALUES ($1, 'Покупки')`, owner)
	before := migrationScalar(t, pool, recipePreviousData)
	// Поздний конфликт: до него миграция создаёт все четыре таблицы и последовательность.
	migrationExec(t, pool, `CREATE TABLE recipe_idempotency_expires_at_idx (sentinel text NOT NULL)`)
	migrationExec(t, pool, `INSERT INTO recipe_idempotency_expires_at_idx VALUES ('Сохранить')`)
	recipeMigrationReject(t, pool, "42P07", recipeMigrationSQL(t, "up"))
	if migrationScalar(t, pool, recipePreviousData) != before {
		t.Fatal("Неудачная миграция изменила существующие данные")
	}
	for _, relation := range []string{
		"recipes", "recipes_id_seq", "recipes_pkey", "recipes_owner_id_id_idx",
		"recipe_ingredients", "recipe_ingredients_pkey", "recipe_steps", "recipe_steps_pkey",
		"recipe_idempotency", "recipe_idempotency_pkey",
	} {
		if migrationScalar(t, pool, `SELECT (to_regclass($1) IS NULL)::text`, relation) != "true" {
			t.Fatalf("Неудачная миграция оставила объект %s", relation)
		}
	}
	if migrationScalar(t, pool, `SELECT sentinel FROM recipe_idempotency_expires_at_idx`) != "Сохранить" {
		t.Fatal("Неудачная миграция изменила существовавший объект")
	}
	// Удаляется только помеха этого теста в его собственной изолированной схеме.
	migrationExec(t, pool, `DROP TABLE recipe_idempotency_expires_at_idx`)
	migrationExec(t, pool, recipeMigrationSQL(t, "up"))
	if migrationScalar(t, pool, recipePreviousData) != before {
		t.Fatal("Повтор миграции после устранения причины сбоя изменил прежние данные")
	}
	recipeMigrationCreate(t, pool, owner)
}

func TestRecipeMigrationDownRefusesWithoutDataLoss(t *testing.T) {
	t.Parallel()
	pool, owner, recipe := recipeMigrationFixture(t)
	migrationExec(t, pool, `INSERT INTO recipe_ingredients (recipe_id, position, name) VALUES ($1, 1, 'Яйца')`, recipe)
	migrationExec(t, pool, `INSERT INTO recipe_steps (recipe_id, position, text) VALUES ($1, 1, 'Приготовить')`, recipe)
	migrationExec(t, pool, `
		INSERT INTO recipe_idempotency (owner_id, key, request_hash, resource_id, response, expires_at)
		VALUES ($1, 'recipe-down-action-01', repeat('a', 64), $2::bigint, jsonb_build_object('id', $2::bigint::text),
		        clock_timestamp() + interval '1 day')`, owner, recipe)
	before := migrationScalar(t, pool, recipeCurrentData)
	previous := migrationScalar(t, pool, recipePreviousData)
	recipeMigrationReject(t, pool, "0A000", recipeMigrationSQL(t, "down"))
	if migrationScalar(t, pool, recipeCurrentData) != before || migrationScalar(t, pool, recipePreviousData) != previous {
		t.Fatal("Отказ down изменил рецепты, replay, последовательности или прежние данные")
	}
}
