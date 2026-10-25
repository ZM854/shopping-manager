//go:build integration

package integration_test

import (
	"os"
	"testing"

	"github.com/ZM854/shopping-manager/backend/internal/testutil/testdb"
)

func stepIDsMigrationSQL(t *testing.T, direction string) string {
	t.Helper()
	data, err := os.ReadFile("../migrations/000011_recipe_step_ids." + direction + ".sql")
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestStepIDsMigrationFreshAndConstraints(t *testing.T) {
	pool := testdb.New(t)
	for version := 0; version < 11; version++ {
		testdb.ApplyMigrations(t, pool, version, version+1)
	}
	owner := migrationUser(t, pool, "step-ids-fresh@example.test")
	id := recipeMigrationCreate(t, pool, owner)
	first := migrationScalar(t, pool, `
		INSERT INTO recipe_steps (recipe_id, position, text) VALUES ($1, 1, 'Первый') RETURNING id::text`, id)
	second := migrationScalar(t, pool, `
		INSERT INTO recipe_steps (recipe_id, position, text) VALUES ($1, 2, 'Второй') RETURNING id::text`, id)
	if first != "1" || second != "2" {
		t.Fatal("Новые ID не назначены последовательностью")
	}
	if migrationScalar(t, pool, `
		SELECT (condeferrable AND condeferred)::text FROM pg_constraint
		WHERE conrelid = 'recipe_steps'::regclass AND conname = 'recipe_steps_recipe_position_key'`) != "true" {
		t.Fatal("Порядок не проверяется в конце транзакции")
	}
	// A swap necessarily passes through a duplicate position inside the transaction.
	migrationExec(t, pool, `DO $$ BEGIN
		UPDATE recipe_steps SET position = 2 WHERE id = 1;
		UPDATE recipe_steps SET position = 1 WHERE id = 2;
	END $$;`)
	if migrationScalar(t, pool, `SELECT text FROM recipe_steps WHERE recipe_id=$1 AND position=1`, id) != "Второй" {
		t.Fatal("Перестановка не сохранилась")
	}
	recipeMigrationReject(t, pool, "23505", `UPDATE recipe_steps SET position=1 WHERE id=1`)
	recipeMigrationReject(t, pool, "23514", `UPDATE recipe_steps SET id=0 WHERE id=1`)
	recipeMigrationReject(t, pool, "23505", `UPDATE recipe_steps SET id=2 WHERE id=1`)
	recipeMigrationReject(t, pool, "23503", `INSERT INTO recipe_steps (recipe_id,position,text) VALUES (99999,1,'Нет')`)
	migrationExec(t, pool, `DELETE FROM recipes WHERE id=$1`, id)
	if migrationScalar(t, pool, `SELECT count(*)::text FROM recipe_steps`) != "0" {
		t.Fatal("Новый PK нарушил каскад удаления шагов")
	}
}

func TestStepIDsMigrationFailureAndDownAreAtomic(t *testing.T) {
	pool, owner, id := recipeMigrationFixture(t)
	migrationExec(t, pool, `INSERT INTO recipe_steps (recipe_id,position,text) VALUES ($1,1,'Сохранить')`, id)
	before := migrationScalar(t, pool, recipeCurrentData)
	// A conflicting constraint makes ALTER fail after earlier actions in the same block.
	migrationExec(t, pool, `ALTER TABLE recipe_steps ADD CONSTRAINT recipe_steps_recipe_id_key CHECK (true)`)
	recipeMigrationReject(t, pool, "42710", stepIDsMigrationSQL(t, "up"))
	if migrationScalar(t, pool, recipeCurrentData) != before ||
		migrationScalar(t, pool, `SELECT (to_regclass('recipe_steps_id_seq') IS NULL)::text`) != "true" ||
		migrationScalar(t, pool, `SELECT count(*)::text FROM information_schema.columns
			WHERE table_schema=current_schema() AND table_name='recipe_steps' AND column_name='id'`) != "0" {
		t.Fatal("Поздний отказ оставил частичную схему или потерял данные")
	}
	migrationExec(t, pool, `ALTER TABLE recipe_steps DROP CONSTRAINT recipe_steps_recipe_id_key`)
	testdb.ApplyMigrations(t, pool, 10, 11)
	beforeDown := migrationScalar(t, pool, recipeCurrentData)
	recipeMigrationReject(t, pool, "0A000", stepIDsMigrationSQL(t, "down"))
	if migrationScalar(t, pool, recipeCurrentData) != beforeDown {
		t.Fatal("Down изменил данные")
	}
	migrationExec(t, pool, `DELETE FROM users WHERE id=$1`, owner)
	if migrationScalar(t, pool, `SELECT count(*)::text FROM recipe_steps`) != "0" {
		t.Fatal("Каскад владельца сломан")
	}
}
