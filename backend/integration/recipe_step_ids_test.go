//go:build integration

package integration_test

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"testing"

	"github.com/ZM854/shopping-manager/backend/internal/recipe"
)

func stepRequest(step recipe.Step, text string) recipe.StepRequest {
	id := strconv.FormatInt(step.ID, 10)
	return recipe.StepRequest{ID: &id, Text: text}
}

func TestRecipeStepIDsPreserveRowsAndRejectForeignIDs(t *testing.T) {
	f := newRecipeRepositoryFixture(t)
	s := recipe.NewService(f.repo)
	ctx := context.Background()
	migrationExec(t, f.pool, `SELECT setval('recipe_steps_id_seq', 9007199254740992)`)
	req := recipeRepositoryRequest("Исходный")
	original, err := s.CreateRecipe(ctx, f.first, "steps-create-main-0001", req)
	if err != nil || original.Steps[0].ID != 9007199254740993 {
		t.Fatal("Не создан точный BIGINT шага", err)
	}
	other := f.create(t, f.first, recipeRepositoryRequest("Другой свой"))
	foreign := f.create(t, f.second, recipeRepositoryRequest("Чужой"))
	migrationExec(t, f.pool, `
		CREATE FUNCTION forbid_step_delete() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN RAISE EXCEPTION 'existing step was deleted'; END $$;
		CREATE TRIGGER forbid_step_delete BEFORE DELETE ON recipe_steps
		FOR EACH ROW EXECUTE FUNCTION forbid_step_delete();`)
	changed := req
	changed.Title = "После перестановки"
	changed.Steps = []recipe.StepRequest{
		stepRequest(original.Steps[1], "Второй изменён"), stepRequest(original.Steps[0], "Первый изменён"),
	}
	updated, err := s.UpdateRecipe(ctx, f.first, original.ID, changed)
	if err != nil || updated.Steps[0].ID != original.Steps[1].ID ||
		updated.Steps[1].ID != original.Steps[0].ID || updated.Steps[0].Position != 1 ||
		updated.Steps[1].Text != "Первый изменён" {
		t.Fatal("PUT удалил строки или изменил ID при правке/перестановке", err)
	}
	migrationExec(t, f.pool, `DROP TRIGGER forbid_step_delete ON recipe_steps`)
	changed.Steps = []recipe.StepRequest{{Text: "Новый"}, stepRequest(updated.Steps[0], "Сохранённый")}
	updated, err = s.UpdateRecipe(ctx, f.first, original.ID, changed)
	if err != nil || updated.Steps[0].ID <= 0 || updated.Steps[0].ID == original.Steps[0].ID ||
		updated.Steps[0].ID == original.Steps[1].ID || updated.Steps[1].ID != original.Steps[1].ID {
		t.Fatal("Новый шаг не получил отдельную идентичность", err)
	}
	if migrationScalar(t, f.pool, `SELECT count(*)::text FROM recipe_steps WHERE id=$1`, original.Steps[0].ID) != "0" {
		t.Fatal("Пропущенный шаг не удалён")
	}
	for _, step := range []recipe.Step{
		{ID: 9223372036854775807}, original.Steps[0], other.Steps[0], foreign.Steps[0],
	} {
		bad := changed
		bad.Title = "Не должно сохраниться"
		bad.Steps = []recipe.StepRequest{stepRequest(step, "Не должно измениться")}
		got, err := s.UpdateRecipe(ctx, f.first, original.ID, bad)
		if !errors.Is(err, recipe.ErrNotFound) || got.ID != 0 {
			t.Fatal("Неизвестный/удалённый/чужой ID шага принят", err)
		}
		after, err := s.GetRecipe(ctx, f.first, original.ID)
		if err != nil || !reflect.DeepEqual(after, updated) {
			t.Fatal("Отказ не откатил рецепт, ингредиенты, шаги или updatedAt", err)
		}
	}
	for _, target := range []struct {
		owner    int64
		original recipe.Recipe
	}{{f.first, other}, {f.second, foreign}} {
		loaded, err := s.GetRecipe(ctx, target.owner, target.original.ID)
		if err != nil || !reflect.DeepEqual(loaded, target.original) {
			t.Fatal("Попытка подмены ID изменила другой рецепт", err)
		}
	}
	replayed, err := s.CreateRecipe(ctx, f.first, "steps-create-main-0001", req)
	if err != nil || !reflect.DeepEqual(replayed, original) {
		t.Fatal("Replay потерял исходные ID удалённых/изменённых шагов", err)
	}
}

func TestRecipeStepIDsHTTPAndReplay(t *testing.T) {
	f := newRecipeHTTPFixture(t)
	req := recipeRepositoryRequest("ID шагов")
	body := recipeHTTPBody(t, req)
	const key = "step-http-replay-0001"
	original := recipeHTTPResponse[recipe.Recipe](t, f.request(f.first, "POST", "/recipes", body, key), 201)
	for _, step := range original.Steps {
		if step.ID <= 0 {
			t.Fatal("Новый POST не вернул ID")
		}
	}
	path := fmt.Sprintf("/recipes/%d", original.ID)
	changed := req
	changed.Steps = []recipe.StepRequest{stepRequest(original.Steps[1], "Сначала второй"), stepRequest(original.Steps[0], "Теперь первый")}
	updated := recipeHTTPResponse[recipe.Recipe](t, f.request(f.first, "PUT", path, recipeHTTPBody(t, changed), ""), 200)
	if updated.Steps[0].ID != original.Steps[1].ID || updated.Steps[1].ID != original.Steps[0].ID {
		t.Fatal("HTTP PUT потерял ID")
	}
	duplicate := changed
	duplicate.Steps = []recipe.StepRequest{changed.Steps[0], changed.Steps[0]}
	validation := recipeHTTPError(t, f.request(f.first, "PUT", path, recipeHTTPBody(t, duplicate), ""), 400, "validation_failed")
	if validation.FieldErrors["steps.1.id"] == "" {
		t.Fatal("Нет пути повторяющегося ID")
	}
	badID := "9223372036854775807"
	bad := changed
	bad.Steps = []recipe.StepRequest{{ID: &badID, Text: "Нет"}}
	recipeHTTPError(t, f.request(f.first, "PUT", path, recipeHTTPBody(t, bad), ""), 404, "not_found")
	recipeHTTPError(t, f.request(f.first, "POST", "/recipes", recipeHTTPBody(t, changed), "step-post-id-0001"), 400, "invalid_request")
	replayed := recipeHTTPResponse[recipe.Recipe](t, f.request(f.first, "POST", "/recipes", body, key), 201)
	if !reflect.DeepEqual(replayed, original) {
		t.Fatal("HTTP replay изменил исходные ID/порядок шагов")
	}
	loaded := recipeHTTPResponse[recipe.Recipe](t, f.request(f.first, "GET", path, "", ""), 200)
	if !reflect.DeepEqual(loaded, updated) {
		t.Fatal("Replay изменил актуальный рецепт")
	}
}
