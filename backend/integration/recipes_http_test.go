//go:build integration

package integration_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ZM854/shopping-manager/backend/internal/auth"
	"github.com/ZM854/shopping-manager/backend/internal/config"
	"github.com/ZM854/shopping-manager/backend/internal/middleware"
	"github.com/ZM854/shopping-manager/backend/internal/recipe"
	"github.com/ZM854/shopping-manager/backend/internal/router"
	"github.com/ZM854/shopping-manager/backend/internal/shopping"
	"github.com/gin-gonic/gin"
)

type recipeHTTPFixture struct {
	*recipeRepositoryFixture
	engine  *gin.Engine
	logs    bytes.Buffer
	tokens  map[int64]string
	refresh string
}

func newRecipeHTTPFixture(t *testing.T) *recipeHTTPFixture {
	t.Helper()
	f := &recipeHTTPFixture{recipeRepositoryFixture: newRecipeRepositoryFixture(t), tokens: make(map[int64]string)}
	log := slog.New(slog.NewJSONHandler(&f.logs, nil))
	tokens := auth.NewTokenService(log, nil,
		"recipe-http-access-secret-at-least-32-bytes",
		"recipe-http-refresh-secret-at-least-32-bytes", time.Hour, time.Hour)
	for _, owner := range []int64{f.first, f.second} {
		pair, err := tokens.GenerateTokens(owner)
		if err != nil {
			t.Fatal(err)
		}
		f.tokens[owner] = pair.AccessToken
		f.refresh = pair.RefreshToken
	}
	f.engine = router.New(
		log, shopping.NewHandler(nil, log), recipe.NewHandler(recipe.NewService(f.repo), log),
		auth.NewAuthHandler(log, nil, false), middleware.NewAuthMiddleware(tokens),
		config.Config{AllowedOrigins: []string{"http://client.example.test"}},
	)
	return f
}

func (f *recipeHTTPFixture) request(owner int64, method, path, body, key string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, "/api"+path, strings.NewReader(body))
	req.ContentLength = -1
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://client.example.test")
	if owner != 0 {
		req.Header.Set("Authorization", "Bearer "+f.tokens[owner])
	}
	if key != "" {
		req.Header.Set("Idempotency-Key", key)
	}
	w := httptest.NewRecorder()
	f.engine.ServeHTTP(w, req)
	return w
}

func recipeHTTPResponse[T any](t *testing.T, w *httptest.ResponseRecorder, status int) T {
	t.Helper()
	if w.Code != status || w.Header().Get("Cache-Control") != "no-store" ||
		len(w.Header().Values("Set-Cookie")) != 0 {
		t.Fatalf("Неверный ответ: %d %s, headers=%v", w.Code, w.Body.String(), w.Header())
	}
	var result T
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func recipeHTTPError(t *testing.T, w *httptest.ResponseRecorder, status int, code string) recipe.ErrorResponse {
	t.Helper()
	result := recipeHTTPResponse[recipe.ErrorResponse](t, w, status)
	if result.Code != code || result.Message == "" {
		t.Fatalf("Неверная ошибка: %s", w.Body.String())
	}
	return result
}

func recipeHTTPBody(t *testing.T, req recipe.SaveRequest) string {
	t.Helper()
	body, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

func TestRecipeHTTPCRUDAndReplay(t *testing.T) {
	f := newRecipeHTTPFixture(t)
	for _, path := range []string{"/recipe-categories", "/recipes", "/recipes/1"} {
		recipeHTTPError(t, f.request(0, "GET", path, "", ""), 401, "unauthorized")
	}
	categories := recipeHTTPResponse[[]recipe.Category](t, f.request(f.first, "GET", "/recipe-categories", "", ""), 200)
	if !reflect.DeepEqual(categories, recipe.Categories()) {
		t.Fatal("Справочник расходится с моделью")
	}
	items := recipeHTTPResponse[[]recipe.Summary](t, f.request(f.first, "GET", "/recipes", "", ""), 200)
	if items == nil || len(items) != 0 {
		t.Fatal("Новый пользователь должен получить []")
	}
	req := recipeRepositoryRequest("Оригинал")
	req.Steps = []recipe.StepRequest{{Text: strings.Repeat("я", recipe.MaxStepTextLength)}}
	body, key := recipeHTTPBody(t, req), "recipe-http-create-0001"
	first := recipeHTTPResponse[recipe.Recipe](t, f.request(f.first, "POST", "/recipes", body, key), 201)
	checkRecipeContents(t, first, req)
	path := fmt.Sprintf("/recipes/%d", first.ID)
	loaded := recipeHTTPResponse[recipe.Recipe](t, f.request(f.first, "GET", path, "", ""), 200)
	if !reflect.DeepEqual(loaded, first) {
		t.Fatal("GET расходится с POST")
	}
	for _, method := range []string{"GET", "PUT", "DELETE"} {
		recipeHTTPError(t, f.request(f.second, method, path, body, ""), 404, "not_found")
	}
	second := recipeHTTPResponse[recipe.Recipe](t, f.request(f.first, "POST", "/recipes", body, "recipe-http-create-0002"), 201)
	items = recipeHTTPResponse[[]recipe.Summary](t, f.request(f.first, "GET", "/recipes", "", ""), 200)
	if !reflect.DeepEqual(items, []recipe.Summary{first.Summary, second.Summary}) {
		t.Fatal("Коллекция не краткая/не упорядочена")
	}
	changed := recipeRepositoryRequest("После правки")
	changed.Ingredients = []recipe.IngredientRequest{{Name: "Соль", Unit: "по вкусу"}, {Name: "Вода"}}
	changed.Steps = []recipe.StepRequest{{Text: "Вскипятить"}, {Text: "Посолить"}}
	updated := recipeHTTPResponse[recipe.Recipe](t, f.request(f.first, "PUT", path, recipeHTTPBody(t, changed), ""), 200)
	checkRecipeContents(t, updated, changed)
	if updated.ID != first.ID || !updated.CreatedAt.Equal(first.CreatedAt) || !updated.UpdatedAt.After(first.UpdatedAt) {
		t.Fatal("PUT неверно изменил ID/время")
	}
	replayed := recipeHTTPResponse[recipe.Recipe](t, f.request(f.first, "POST", "/recipes", body, key), 201)
	if !reflect.DeepEqual(replayed, first) {
		t.Fatal("Повтор после правки должен вернуть исходный DTO")
	}
	recipeHTTPError(t, f.request(f.first, "POST", "/recipes", recipeHTTPBody(t, changed), key), 409, "idempotency_conflict")
	w := f.request(f.first, "DELETE", path, "", "")
	if w.Code != 204 || w.Body.Len() != 0 || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("DELETE: %d %s", w.Code, w.Body.String())
	}
	recipeHTTPError(t, f.request(f.first, "GET", path, "", ""), 404, "not_found")
	recipeHTTPError(t, f.request(f.first, "PUT", path, body, ""), 404, "not_found")
	recipeHTTPError(t, f.request(f.first, "DELETE", path, "", ""), 404, "not_found")
	recipeHTTPError(t, f.request(f.first, "POST", "/recipes", body, key), 409, "resource_deleted")
	newRecipe := recipeHTTPResponse[recipe.Recipe](t, f.request(f.first, "POST", "/recipes", body, "recipe-http-create-0003"), 201)
	if newRecipe.ID == first.ID {
		t.Fatal("Новое действие восстановило удалённый ID")
	}
}

func TestRecipeHTTPValidationAndLimits(t *testing.T) {
	f := newRecipeHTTPFixture(t)
	body := recipeHTTPBody(t, recipeRepositoryRequest("Пирог"))
	before := migrationScalar(t, f.pool, recipeServiceState)
	for _, tc := range []struct{ body, key, code string }{
		{body, "", "validation_failed"}, {body, "short", "validation_failed"},
		{`{"title":null}`, "recipe-http-invalid-0001", "invalid_request"},
		{`{"ingredients":[{"name":"one","name":"two"}]}`, "recipe-http-invalid-0001", "invalid_request"},
		{`{"steps":[{"text":"one","text":"two"}]}`, "recipe-http-invalid-0001", "invalid_request"},
		{`{"steps":[{"text":"one","position":1}]}`, "recipe-http-invalid-0001", "invalid_request"},
		{strings.Replace(body, "Пирог", string([]byte{0xff}), 1), "recipe-http-invalid-0001", "invalid_request"},
		{strings.Replace(body, `"quantity":"999999999.999"`, `"quantity":"0"`, 1), "recipe-http-invalid-0001", "validation_failed"},
	} {
		recipeHTTPError(t, f.request(f.first, "POST", "/recipes", tc.body, tc.key), 400, tc.code)
	}
	if migrationScalar(t, f.pool, recipeServiceState) != before {
		t.Fatal("Невалидные HTTP-запросы изменили БД")
	}
	for _, method := range []string{"POST", "PUT"} {
		path := "/recipes"
		if method == "PUT" {
			path += "/1"
		}
		oversized := body + strings.Repeat(" ", recipe.MaxJSONBodyBytes+1-len(body))
		recipeHTTPError(t, f.request(f.first, method, path, oversized, "recipe-http-large-0001"), 413, "body_too_large")
	}
	if migrationScalar(t, f.pool, recipeServiceState) != before {
		t.Fatal("Превышение тела создало частичные данные")
	}
	// Отсутствующие quantity/unit разрешены и явно присутствуют в ответе.
	small := `{"title":"Суп","cookingTimeMinutes":5,"categoryCode":"soups","ingredients":[{"name":"Соль"}],"steps":[{"text":"Сварить"}]}`
	result := recipeHTTPResponse[map[string]json.RawMessage](t, f.request(f.first, "POST", "/recipes", small, "recipe-http-minimal-0001"), 201)
	var ingredients []map[string]json.RawMessage
	if err := json.Unmarshal(result["ingredients"], &ingredients); err != nil || len(ingredients) != 1 ||
		string(ingredients[0]["quantity"]) != "null" || string(ingredients[0]["unit"]) != `""` ||
		len(ingredients[0]) != 4 || len(result) != 8 || result["ownerId"] != nil {
		t.Fatal("Неверная форма публичного DTO", err)
	}
}

func TestRecipeHTTPInternalErrorAndTokenPurpose(t *testing.T) {
	f := newRecipeHTTPFixture(t)
	req := httptest.NewRequest("GET", "/api/recipe-categories", nil)
	req.Header.Set("Authorization", "Bearer "+f.refresh)
	w := httptest.NewRecorder()
	f.engine.ServeHTTP(w, req)
	recipeHTTPError(t, w, 401, "unauthorized")
	migrationExec(t, f.pool, `CREATE FUNCTION fail_recipe_http() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN RAISE EXCEPTION 'private-recipe-dependency-secret'; END $$`)
	migrationExec(t, f.pool, `CREATE TRIGGER fail_recipe_http BEFORE UPDATE ON recipe_idempotency
		FOR EACH ROW EXECUTE FUNCTION fail_recipe_http()`)
	before := migrationScalar(t, f.pool, recipeServiceState)
	body := recipeHTTPBody(t, recipeRepositoryRequest("private-recipe-body-secret"))
	w = f.request(f.first, "POST", "/recipes?private=query-secret", body, "recipe-http-failure-0001")
	recipeHTTPError(t, w, 500, "internal_error")
	if migrationScalar(t, f.pool, recipeServiceState) != before {
		t.Fatal("HTTP 500 оставил рецепт или claim")
	}
	for _, secret := range []string{"private-recipe", "query-secret", f.tokens[f.first], f.refresh} {
		if strings.Contains(f.logs.String(), secret) || strings.Contains(w.Body.String(), secret) {
			t.Fatal("Ошибка или журнал раскрывают данные запроса/зависимости")
		}
	}
	migrationExec(t, f.pool, "DROP TRIGGER fail_recipe_http ON recipe_idempotency")
	recipeHTTPResponse[recipe.Recipe](t, f.request(f.first, "POST", "/recipes", body, "recipe-http-failure-0001"), 201)
}
