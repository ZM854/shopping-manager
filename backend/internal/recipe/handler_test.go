package recipe

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

const handlerBody = `{"title":"Пирог","cookingTimeMinutes":30,"categoryCode":"baking","ingredients":[{"name":"Мука","quantity":"2.500","unit":"г"},{"name":"Соль","quantity":null},{"name":"Вода"}],"steps":[{"text":"Смешать\n\tИспечь"}]}`

type handlerService struct {
	RecipeService
	err    error
	result Recipe
	called string
	owner  int64
	id     int64
	key    string
	req    SaveRequest
}

func (s *handlerService) GetRecipes(_ context.Context, owner int64) ([]Summary, error) {
	s.called, s.owner = "collection", owner
	return []Summary{}, s.err
}

func (s *handlerService) GetRecipe(_ context.Context, owner, id int64) (Recipe, error) {
	s.called, s.owner, s.id = "get", owner, id
	return s.result, s.err
}

func (s *handlerService) CreateRecipe(_ context.Context, owner int64, key string, req SaveRequest) (Recipe, error) {
	s.called, s.owner, s.key, s.req = "create", owner, key, req
	return s.result, s.err
}

func (s *handlerService) UpdateRecipe(_ context.Context, owner, id int64, req SaveRequest) (Recipe, error) {
	s.called, s.owner, s.id, s.req = "update", owner, id, req
	return s.result, s.err
}

func (s *handlerService) DeleteRecipe(_ context.Context, owner, id int64) error {
	s.called, s.owner, s.id = "delete", owner, id
	return s.err
}

func handlerEngine(service RecipeService, user any, log *slog.Logger) *gin.Engine {
	engine := gin.New()
	group := engine.Group("/api")
	group.Use(func(c *gin.Context) {
		if user != nil {
			c.Set("userID", user)
		}
	})
	NewHandler(service, log).Register(group)
	return engine
}

func handlerRequest(engine *gin.Engine, method, path, body string, length int64) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.ContentLength = length
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", "handler-create-0001")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	return w
}

func handlerError(t *testing.T, w *httptest.ResponseRecorder, status int, code string) ErrorResponse {
	t.Helper()
	var result ErrorResponse
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if w.Code != status || result.Code != code || result.Message == "" ||
		w.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("Неверная ошибка: %d %s, headers=%v", w.Code, w.Body.String(), w.Header())
	}
	return result
}

func TestHandlerRoutesAndDTO(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	for _, tc := range []struct {
		method, path, call string
		status             int
	}{
		{"GET", "/api/recipes", "collection", 200},
		{"GET", "/api/recipes/9007199254740993", "get", 200},
		{"POST", "/api/recipes", "create", 201},
		{"PUT", "/api/recipes/9007199254740993", "update", 200},
		{"DELETE", "/api/recipes/9007199254740993", "delete", 204},
	} {
		t.Run(tc.method, func(t *testing.T) {
			s := &handlerService{result: Recipe{Summary: Summary{ID: 9007199254740993}, Ingredients: []Ingredient{}, Steps: []Step{}}}
			w := handlerRequest(handlerEngine(s, int64(7), log), tc.method, tc.path, handlerBody, -1)
			if w.Code != tc.status || s.called != tc.call || s.owner != 7 || w.Header().Get("Cache-Control") != "no-store" {
				t.Fatalf("Маршрут: %d %s, service=%+v", w.Code, w.Body.String(), s)
			}
			if tc.call == "get" || tc.call == "update" || tc.call == "delete" {
				if s.id != 9007199254740993 {
					t.Fatal("Потеряна точность ID")
				}
			}
			if tc.call == "create" || tc.call == "update" {
				if s.req.Ingredients[0].Quantity == nil || *s.req.Ingredients[0].Quantity != "2.500" ||
					s.req.Ingredients[1].Quantity != nil || s.req.Ingredients[2].Quantity != nil ||
					s.req.Ingredients[1].Unit != "" || s.req.Steps[0].Text != "Смешать\n\tИспечь" {
					t.Fatalf("Повреждено тело: %+v", s.req)
				}
			}
			if tc.call == "create" && s.key != "handler-create-0001" {
				t.Fatal("Потерян ключ повтора")
			}
			if tc.call == "delete" && w.Body.Len() != 0 {
				t.Fatal("DELETE вернул тело")
			}
			if tc.call == "collection" && w.Body.String() != "[]" {
				t.Fatal("Пустая коллекция не []")
			}
			if tc.call == "get" && !strings.Contains(w.Body.String(), `"id":"9007199254740993"`) {
				t.Fatal("ID DTO не строка")
			}
		})
	}
	w := handlerRequest(handlerEngine(nil, int64(7), log), "GET", "/api/recipe-categories", "", 0)
	var categories []Category
	if err := json.Unmarshal(w.Body.Bytes(), &categories); err != nil || w.Code != 200 || !reflect.DeepEqual(categories, Categories()) {
		t.Fatal("Неверный справочник категорий", err)
	}
}

func TestHandlerStrictJSON(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	engine := handlerEngine(nil, int64(7), log)
	bodies := []string{
		"", " ", "null", "[]", "true", `"text"`, "{} {}", handlerBody + " false", handlerBody + "!",
		`{"title":"one","title":"two"}`, `{"title":"one","ti\u0074le":"two"}`,
		`{"Title":"one"}`, `{"ownerId":"7"}`, `{"id":"1"}`, `{"createdAt":"now"}`,
		`{"ingredients":[{"name":"one","name":"two"}]}`,
		`{"ingredients":[{"name":"one","na\u006de":"two"}]}`,
		`{"ingredients":[{"Name":"one"}]}`, `{"ingredients":[{"position":1}]}`,
		`{"ingredients":[{"id":"1"}]}`, `{"ingredients":[{"quantity":"1","quantity":null}]}`,
		`{"steps":[{"text":"one","text":"two"}]}`, `{"steps":[{"Text":"one"}]}`,
		`{"steps":[{"position":1}]}`, `{"steps":[{"photo":"url"}]}`,
		`{"title":null}`, `{"cookingTimeMinutes":null}`, `{"categoryCode":null}`,
		`{"ingredients":null}`, `{"steps":null}`, `{"ingredients":[null]}`, `{"steps":[null]}`,
		`{"ingredients":[{"name":null}]}`, `{"ingredients":[{"unit":null}]}`, `{"steps":[{"text":null}]}`,
		`{"ingredients":{}}`, `{"steps":{}}`, `{"ingredients":[[]]}`, `{"steps":[1]}`,
		strings.Replace(handlerBody, `"quantity":"2.500"`, `"quantity":2.5`, 1),
		strings.Replace(handlerBody, `"unit":"г"`, `"unit":false`, 1),
		strings.Replace(handlerBody, `"name":"Мука"`, `"name":{}`, 1),
		strings.Replace(handlerBody, `"text":"Смешать\n\tИспечь"`, `"text":[]`, 1),
		strings.Replace(handlerBody, `"title":"Пирог"`, `"title":3`, 1),
		strings.Replace(handlerBody, `"categoryCode":"baking"`, `"categoryCode":false`, 1),
		strings.Replace(handlerBody, `"cookingTimeMinutes":30`, `"cookingTimeMinutes":"30"`, 1),
		strings.Replace(handlerBody, `"cookingTimeMinutes":30`, `"cookingTimeMinutes":30.0`, 1),
		strings.Replace(handlerBody, `"cookingTimeMinutes":30`, `"cookingTimeMinutes":3e1`, 1),
		strings.Replace(handlerBody, `"cookingTimeMinutes":30`, `"cookingTimeMinutes":9223372036854775808`, 1),
		strings.Replace(handlerBody, "Пирог", string([]byte{0xff}), 1),
		handlerBody[:len(handlerBody)-1], `{"ingredients":[{"name":"unfinished"}`, `{"steps":[{"text":"a"},]}`,
	}
	for i, body := range bodies {
		for _, method := range []string{"POST", "PUT"} {
			path := "/api/recipes"
			if method == "PUT" {
				path += "/1"
			}
			w := handlerRequest(engine, method, path, body, -1)
			if w.Code != 400 {
				t.Fatalf("Случай %d %s: %d %s", i, method, w.Code, w.Body.String())
			}
			handlerError(t, w, 400, "invalid_request")
		}
	}
}

func TestHandlerBodyLimit(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	for _, method := range []string{"POST", "PUT"} {
		path := "/api/recipes"
		if method == "PUT" {
			path += "/1"
		}
		for _, size := range []int{MaxJSONBodyBytes, MaxJSONBodyBytes + 1} {
			for _, length := range []int64{-1, 0, int64(size)} {
				s := &handlerService{}
				body := handlerBody + strings.Repeat(" ", size-len(handlerBody))
				w := handlerRequest(handlerEngine(s, int64(7), log), method, path, body, length)
				if size > MaxJSONBodyBytes {
					handlerError(t, w, 413, "body_too_large")
					if s.called != "" {
						t.Fatal("Превышение тела дошло до service")
					}
				} else if (w.Code != 200 && w.Code != 201) || s.called == "" {
					t.Fatalf("Тело ровно в лимит отвергнуто: %d %s", w.Code, w.Body.String())
				}
			}
		}
	}
}

func TestHandlerValidationAndErrors(t *testing.T) {
	var logs bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&logs, nil))
	for _, tc := range []struct {
		err    error
		status int
		code   string
	}{
		{ErrNotFound, 404, "not_found"}, {ErrIdempotencyConflict, 409, "idempotency_conflict"},
		{ErrResourceDeleted, 409, "resource_deleted"}, {invalid("ingredients.0.quantity"), 400, "validation_failed"},
		{errors.New("private SQL recipe secret"), 500, "internal_error"},
	} {
		s := &handlerService{err: tc.err, result: Recipe{Summary: Summary{ID: 123}}}
		w := handlerRequest(handlerEngine(s, int64(7), log), "POST", "/api/recipes?query-secret=private", handlerBody, -1)
		result := handlerError(t, w, tc.status, tc.code)
		if strings.Contains(w.Body.String(), "123") || strings.Contains(w.Body.String(), "private") {
			t.Fatal("Ошибка содержит DTO успеха или детали зависимости")
		}
		if tc.code == "validation_failed" && result.FieldErrors["ingredients.0.quantity"] == "" {
			t.Fatal("Потерян путь ошибки поля")
		}
	}
	if strings.Contains(logs.String(), "secret") || strings.Contains(logs.String(), "private") ||
		!strings.Contains(logs.String(), "recipe request failed") {
		t.Fatal("Небезопасная или отсутствующая диагностика HTTP")
	}
	engine := handlerEngine(NewService(nil), int64(7), log)
	for _, body := range []string{"{}", `{"ingredients":[{}]}`, `{"steps":[{}]}`,
		strings.Replace(handlerBody, `"name":"Мука"`, `"name":""`, 1),
		strings.Replace(handlerBody, `"text":"Смешать\n\tИспечь"`, `"text":""`, 1),
		`{"ingredients":[` + strings.TrimSuffix(strings.Repeat(`{},`, MaxIngredients+1), ",") + `]}`,
		`{"steps":[` + strings.TrimSuffix(strings.Repeat(`{},`, MaxSteps+1), ",") + `]}`,
	} {
		handlerError(t, handlerRequest(engine, "POST", "/api/recipes", body, -1), 400, "validation_failed")
	}
}

func TestHandlerAuthAndIDs(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	for _, user := range []any{nil, "7", int64(0), int64(-1)} {
		engine := handlerEngine(nil, user, log)
		for _, path := range []string{"/api/recipes", "/api/recipes/1", "/api/recipe-categories"} {
			handlerError(t, handlerRequest(engine, "GET", path, "", 0), 401, "unauthorized")
		}
	}
	engine := handlerEngine(nil, int64(7), log)
	for _, id := range []string{"0", "-1", "01", "+1", "1.0", "9223372036854775808", "text"} {
		for _, method := range []string{"GET", "PUT", "DELETE"} {
			handlerError(t, handlerRequest(engine, method, "/api/recipes/"+id, handlerBody, -1), 400, "validation_failed")
		}
	}
}
