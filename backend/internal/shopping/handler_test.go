package shopping

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

type forbiddenStore struct {
	Store
	t *testing.T
}

func (f *forbiddenStore) InTx(_ context.Context, _ func(Transaction) error) error {
	f.t.Fatal("Invalid request reached storage")
	return nil
}

func handlerEngine(service ShoppingService, user any) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	group := r.Group("/api")
	group.Use(func(c *gin.Context) {
		if user != nil {
			c.Set("userID", user)
		}
	})
	NewHandler(service, slog.New(slog.NewTextHandler(io.Discard, nil))).Register(group)
	return r
}
func handlerRequest(r *gin.Engine, method, path, body, key string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, "/api"+path, strings.NewReader(body))
	req.Header.Set("Idempotency-Key", key)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestHTTPRejectsMalformedJSONAndInvalidFields(t *testing.T) {
	r := handlerEngine(NewService(&forbiddenStore{t: t}), int64(7))
	routes := []struct{ method, path string }{
		{"POST", "/shopping-lists"}, {"PATCH", "/shopping-lists/1"},
		{"POST", "/shopping-lists/1/items"}, {"PATCH", "/shopping-lists/1/items/2"},
		{"PATCH", "/shopping-lists/1/items/mark-all"},
	}
	for _, route := range routes {
		for _, body := range []string{"", "null", "[]", "1", "true", "{", `{"unknown":true}`, `{"name":null}`, `{"name":"A","name":"B"}`, `{"Name":"A"}`, `{} {}`, "{\"name\":\"\xff\"}"} {
			t.Run(route.path+body, func(t *testing.T) {
				w := handlerRequest(r, route.method, route.path, body, "valid-action-0001")
				if w.Code != 400 || !strings.Contains(w.Body.String(), `"code":"invalid_request"`) {
					t.Fatalf("Malformed JSON: %d %s", w.Code, w.Body.String())
				}
			})
		}
		w := handlerRequest(r, route.method, route.path, `{"name":"`+strings.Repeat("x", MaxJSONBodyBytes)+`"}`, "valid-action-0001")
		if w.Code != 413 || !strings.Contains(w.Body.String(), "body_too_large") {
			t.Fatal("Body limit not enforced", w.Code)
		}
	}
	for _, tc := range []struct{ method, path, body, key string }{
		{"POST", "/shopping-lists", `{}`, "valid-action-0001"},
		{"POST", "/shopping-lists", `{"name":"A"}`, ""},
		{"POST", "/shopping-lists", `{"name":"A"}`, "short"},
		{"POST", "/shopping-lists", `{"name":"A"}`, strings.Repeat("x", 129)},
		{"PATCH", "/shopping-lists/1", `{}`, ""},
		{"PATCH", "/shopping-lists/1", `{"name":" "}`, ""},
		{"POST", "/shopping-lists/1/items", `{"name":"A","quantity":1}`, "valid-action-0001"},
		{"POST", "/shopping-lists/1/items", `{"name":"A","unit":null}`, "valid-action-0001"},
		{"POST", "/shopping-lists/1/items", `{"name":"A","isMarked":null}`, "valid-action-0001"},
		{"POST", "/shopping-lists/1/items", `{"name":"A","quantity":"1e3"}`, "valid-action-0001"},
		{"POST", "/shopping-lists/1/items", `{"name":"A","user_id":"7"}`, "valid-action-0001"},
		{"POST", "/shopping-lists/1/items", `{}`, "valid-action-0001"},
		{"PATCH", "/shopping-lists/1/items/2", `{}`, ""},
		{"PATCH", "/shopping-lists/1/items/2", `{"unit":null}`, ""},
		{"PATCH", "/shopping-lists/1/items/2", `{"isMarked":null}`, ""},
		{"PATCH", "/shopping-lists/1/items/2", `{"quantity":0}`, ""},
		{"PATCH", "/shopping-lists/1/items/mark-all", `{}`, ""},
		{"PATCH", "/shopping-lists/1/items/mark-all", `{"isMarked":"true"}`, ""},
	} {
		w := handlerRequest(r, tc.method, tc.path, tc.body, tc.key)
		if w.Code != 400 {
			t.Fatalf("Invalid fields reached storage: %+v %d %s", tc, w.Code, w.Body.String())
		}
	}
}

func TestHTTPRejectsInvalidIDsAndMissingUser(t *testing.T) {
	r := handlerEngine(nil, int64(7))
	for _, id := range []string{"0", "-1", "01", "+1", "9223372036854775808", "abc"} {
		for _, path := range []string{"/shopping-lists/" + id, "/shopping-lists/1/items/" + id} {
			w := handlerRequest(r, "GET", path, "", "")
			if w.Code != 400 {
				t.Fatalf("ID %q status=%d", id, w.Code)
			}
		}
	}
	for _, user := range []any{nil, "7", int64(0), int64(-1)} {
		w := handlerRequest(handlerEngine(nil, user), "GET", "/shopping-lists", "", "")
		if w.Code != 401 || !strings.Contains(w.Body.String(), "unauthorized") {
			t.Fatal("Invalid user passed", user)
		}
	}
}

type failingService struct {
	ShoppingService
	err error
}

func (s failingService) GetList(context.Context, int64, int64) (List, error) { return List{}, s.err }
func TestHTTPErrorMappingHidesInternalDetails(t *testing.T) {
	for _, tc := range []struct {
		err    error
		status int
		code   string
	}{
		{ErrNotFound, 404, "not_found"}, {ErrIdempotencyConflict, 409, "idempotency_conflict"},
		{ErrResourceDeleted, 409, "resource_deleted"}, {invalid("name"), 400, "validation_failed"},
		{errors.New("SQL secret-test-value"), 500, "internal_error"},
	} {
		w := handlerRequest(handlerEngine(failingService{err: tc.err}, int64(7)), "GET", "/shopping-lists/9007199254740993", "", "")
		if w.Code != tc.status || !strings.Contains(w.Body.String(), tc.code) || strings.Contains(w.Body.String(), "secret-test-value") {
			t.Fatalf("Error mapping: %d %s", w.Code, w.Body.String())
		}
	}
}

type failingBody struct{}

func (failingBody) Read([]byte) (int, error) { return 0, errors.New("read failed") }
func (failingBody) Close() error             { return nil }
func TestHTTPBodyReadFailure(t *testing.T) {
	r := handlerEngine(nil, int64(7))
	req := httptest.NewRequest("POST", "/api/shopping-lists", nil)
	req.Body = failingBody{}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != 400 {
		t.Fatal("Body read failure not handled")
	}
}

func TestHTTPAcceptsExactlyMaxBodyBytes(t *testing.T) {
	tx := &replayTransaction{}
	r := handlerEngine(NewService(transactionStore{tx: tx}), int64(7))
	body := `{"name":"A"}`
	body += strings.Repeat(" ", MaxJSONBodyBytes-len(body))
	w := handlerRequest(r, "POST", "/shopping-lists", body, "body-boundary-001")
	if w.Code != 201 {
		t.Fatalf("Body at limit rejected: %d %s", w.Code, w.Body.String())
	}
	w = handlerRequest(r, "POST", "/shopping-lists", body+" ", "body-boundary-001")
	if w.Code != 413 {
		t.Fatal("Body above limit accepted")
	}
}
