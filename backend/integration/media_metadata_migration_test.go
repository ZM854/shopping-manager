//go:build integration

package integration_test

import (
	"context"
	"os"
	"reflect"
	"testing"

	"github.com/ZM854/shopping-manager/backend/internal/media"
	"github.com/ZM854/shopping-manager/backend/internal/recipe"
	"github.com/ZM854/shopping-manager/backend/internal/testutil/testdb"
)

func mediaMigrationSQL(t *testing.T, direction string) string {
	t.Helper()
	b, err := os.ReadFile("../migrations/000012_recipe_media." + direction + ".sql")
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestMediaMetadataMigrationFreshAndChecks(t *testing.T) {
	pool := testdb.New(t)
	for version := 0; version < 12; version++ {
		testdb.ApplyMigrations(t, pool, version, version+1)
	}
	owner := migrationUser(t, pool, "media-schema@example.test")
	s := media.NewService(media.NewRepository(pool))
	r := reserveMedia(t, s, owner, "media-migration-0001", media.StepVideo, 50<<20)
	if migrationScalar(t, pool, `SELECT count(*)::text FROM recipe_media_variants WHERE media_id=$1`, r.ID) != "2" {
		t.Fatal("Missing planned keys")
	}
	for _, query := range []string{
		`UPDATE recipe_media SET state='unknown'`, `UPDATE recipe_media SET purpose='avatar'`,
		`UPDATE recipe_media SET byte_size=52428801`, `UPDATE recipe_media SET source_reserved_bytes=1`,
		`UPDATE recipe_media SET accounted_bytes=0`, `UPDATE recipe_media SET owner_id=NULL`,
		`UPDATE recipe_media SET state='failed',error_code='raw private stderr'`,
		`UPDATE recipe_media SET state='failed',error_code=NULL`, `UPDATE recipe_media SET state='attached'`,
		`UPDATE recipe_media_variants SET name='original' WHERE name='video'`,
		`UPDATE recipe_media_variants SET object_key='../../private.mp4' WHERE name='video'`,
		`UPDATE recipe_media_variants SET width=1280 WHERE name='video'`,
		`UPDATE recipe_media_variants SET content_type='video/mp4',byte_size=20971521,width=1280,height=720,duration_milliseconds=1000 WHERE name='video'`,
		`UPDATE recipe_media_variants SET content_type='video/mp4',byte_size=100,width=1281,height=720,duration_milliseconds=1000 WHERE name='video'`,
		`UPDATE recipe_media_variants SET content_type='video/mp4',byte_size=100,width=1280,height=720,duration_milliseconds=60001 WHERE name='video'`,
	} {
		recipeMigrationReject(t, pool, "23514", query)
	}
	readyMedia(t, s, owner, r)
	before := migrationScalar(t, pool, `SELECT string_agg(object_key,',' ORDER BY object_key) FROM recipe_media_variants`)
	recipeMigrationReject(t, pool, "0A000", mediaMigrationSQL(t, "down"))
	if migrationScalar(t, pool, `SELECT string_agg(object_key,',' ORDER BY object_key) FROM recipe_media_variants`) != before {
		t.Fatal("Down destroyed keys")
	}
}

func TestMediaMetadataMigrationPreservesCurrentRecipesAndIsAtomic(t *testing.T) {
	pool := testdb.New(t)
	testdb.ApplyMigrations(t, pool, 0, 11)
	owner := migrationUser(t, pool, "media-existing@example.test")
	s := recipe.NewService(recipe.NewRepository(pool))
	ctx := context.Background()
	r, err := s.CreateRecipe(ctx, owner, "media-existing-recipe-0001", recipeRepositoryRequest("Текущий рецепт с ID"))
	if err != nil {
		t.Fatal(err)
	}
	before := migrationScalar(t, pool, recipeCurrentData)
	// Failure near the end of DDL must also roll back the parent unique key and sequences.
	migrationExec(t, pool, `CREATE TABLE recipe_media_idempotency (id integer)`)
	recipeMigrationReject(t, pool, "42P07", mediaMigrationSQL(t, "up"))
	if migrationScalar(t, pool, recipeCurrentData) != before ||
		migrationScalar(t, pool, `SELECT (to_regclass('recipe_media') IS NULL AND to_regclass('recipe_media_variants') IS NULL AND to_regclass('recipe_media_id_seq') IS NULL)::text`) != "true" ||
		migrationScalar(t, pool, `SELECT count(*)::text FROM pg_constraint WHERE conrelid='recipes'::regclass AND conname='recipes_owner_id_id_key'`) != "0" {
		t.Fatal("Partial media migration survived failure")
	}
	migrationExec(t, pool, `DROP TABLE recipe_media_idempotency`)
	testdb.ApplyMigrations(t, pool, 11, 12)
	if migrationScalar(t, pool, recipeCurrentData) != before {
		t.Fatal("Media schema changed existing data")
	}
	repeated, err := s.CreateRecipe(ctx, owner, "media-existing-recipe-0001", recipeRepositoryRequest("Текущий рецепт с ID"))
	if err != nil || !reflect.DeepEqual(r, repeated) {
		t.Fatal("Current recipe replay changed", err)
	}
}
