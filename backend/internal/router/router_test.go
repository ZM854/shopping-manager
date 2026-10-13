package router

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ZM854/shopping-manager/backend/internal/auth"
	"github.com/ZM854/shopping-manager/backend/internal/middleware"
	"github.com/ZM854/shopping-manager/backend/internal/shopping"
)

func TestAPIPrefixAndAuthorization(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	engine := New(
		log,
		shopping.NewHandler(nil, log),
		auth.NewAuthHandler(log, nil, "http://localhost:5173/login"),
		middleware.NewAuthMiddleware(nil),
		"http://localhost:5173",
	)

	for _, test := range []struct {
		method string
		path   string
		status int
	}{
		{http.MethodGet, "/api/healthz", http.StatusOK},
		{http.MethodGet, "/healthz", http.StatusNotFound},
		{http.MethodGet, "/api/shopping-lists", http.StatusUnauthorized},
		{http.MethodPost, "/api/shopping-lists", http.StatusUnauthorized},
		{http.MethodPatch, "/api/shopping-lists/1", http.StatusUnauthorized},
		{http.MethodGet, "/api/shopping-lists/1", http.StatusUnauthorized},
		{http.MethodDelete, "/api/shopping-lists/1", http.StatusUnauthorized},
		{http.MethodGet, "/api/shopping-lists/1/items", http.StatusUnauthorized},
		{http.MethodPost, "/api/shopping-lists/1/items", http.StatusUnauthorized},
		{http.MethodDelete, "/api/shopping-lists/1/items", http.StatusUnauthorized},
		{http.MethodPatch, "/api/shopping-lists/1/items/mark-all", http.StatusUnauthorized},
		{http.MethodGet, "/api/shopping-lists/1/items/2", http.StatusUnauthorized},
		{http.MethodPatch, "/api/shopping-lists/1/items/2", http.StatusUnauthorized},
		{http.MethodDelete, "/api/shopping-lists/1/items/2", http.StatusUnauthorized},
		{http.MethodGet, "/api/products", http.StatusNotFound},
		{http.MethodGet, "/products", http.StatusNotFound},
		// Public auth routes must reach validation without requiring a bearer token.
		{http.MethodPost, "/api/login", http.StatusBadRequest},
		{http.MethodPost, "/login", http.StatusNotFound},
		{http.MethodPost, "/api/registration", http.StatusBadRequest},
		{http.MethodPost, "/registration", http.StatusNotFound},
	} {
		t.Run(test.method+" "+test.path, func(t *testing.T) {
			response := httptest.NewRecorder()
			engine.ServeHTTP(response, httptest.NewRequest(test.method, test.path, nil))
			if response.Code != test.status {
				t.Fatalf("status = %d, want %d", response.Code, test.status)
			}
		})
	}
}

func TestCORSAllowedAndRejectedOrigins(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	engine := New(log, shopping.NewHandler(nil, log), auth.NewAuthHandler(log, nil, "http://client.example.test/login"), middleware.NewAuthMiddleware(nil), "http://client.example.test")
	for _, test := range []struct {
		origin string
		status int
	}{{"http://client.example.test", 204}, {"https://foreign.example.test", 403}} {
		request := httptest.NewRequest(http.MethodOptions, "/api/shopping-lists", nil)
		request.Header.Set("Origin", test.origin)
		request.Header.Set("Access-Control-Request-Method", "POST")
		request.Header.Set("Access-Control-Request-Headers", "Authorization,Content-Type,Idempotency-Key")
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, request)
		if response.Code != test.status {
			t.Fatalf("origin=%s status=%d", test.origin, response.Code)
		}
		if test.status == 204 && (response.Header().Get("Access-Control-Allow-Origin") != test.origin || response.Header().Get("Access-Control-Allow-Credentials") != "true") {
			t.Fatal("Allowed origin lost credentials")
		}
		if test.status == 403 && response.Header().Get("Access-Control-Allow-Origin") != "" {
			t.Fatal("Foreign origin authorized")
		}
	}
}
