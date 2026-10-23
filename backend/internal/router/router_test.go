package router

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ZM854/shopping-manager/backend/internal/auth"
	"github.com/ZM854/shopping-manager/backend/internal/config"
	"github.com/ZM854/shopping-manager/backend/internal/middleware"
	"github.com/ZM854/shopping-manager/backend/internal/recipe"
	"github.com/ZM854/shopping-manager/backend/internal/shopping"
)

func TestRateLimitAndProxyTrust(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	for _, trusted := range []bool{false, true} {
		cfg := config.Config{AllowedOrigins: []string{"http://client.example.test"}, AuthLoginLimit: 2, AuthRegistrationLimit: 2, AuthResendLimit: 2}
		if trusted {
			cfg.TrustedProxies = []string{"127.0.0.1"}
		}
		engine := New(
			log, shopping.NewHandler(nil, log), recipe.NewHandler(nil, log),
			auth.NewAuthHandler(log, nil, false), middleware.NewAuthMiddleware(nil), cfg,
		)
		for index := range 3 {
			req := httptest.NewRequest("POST", "/api/login", nil)
			req.RemoteAddr = "127.0.0.1:12345"
			req.Header.Set("X-Forwarded-For", fmt.Sprintf("203.0.113.%d", index+1))
			response := httptest.NewRecorder()
			engine.ServeHTTP(response, req)
			want := 400
			if !trusted && index == 2 {
				want = 429
			}
			if response.Code != want || want == 429 && response.Header().Get("Retry-After") == "" {
				t.Fatal("IP limiter trusts spoofed headers or loses Retry-After")
			}
		}
		// Registration has its own budget, and GETs do not consume auth limits.
		for _, path := range []string{"/api/registration", "/api/activation/resend"} {
			req := httptest.NewRequest("POST", path, nil)
			req.RemoteAddr = "127.0.0.1:12345"
			response := httptest.NewRecorder()
			engine.ServeHTTP(response, req)
			if response.Code != 400 {
				t.Fatal("Operations share a rate budget")
			}
		}
	}
}

func TestAPIPrefixAndAuthorization(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	engine := New(
		log,
		shopping.NewHandler(nil, log),
		recipe.NewHandler(nil, log),
		auth.NewAuthHandler(log, nil, false),
		middleware.NewAuthMiddleware(nil),
		config.Config{AllowedOrigins: []string{"http://localhost:5173"}, AuthLoginLimit: 1000, AuthRegistrationLimit: 1000, AuthResendLimit: 1000},
	)

	for _, test := range []struct {
		method string
		path   string
		status int
	}{
		{http.MethodGet, "/api/healthz", http.StatusOK},
		{http.MethodGet, "/api/recipe-categories", http.StatusUnauthorized},
		{http.MethodGet, "/api/recipes", http.StatusUnauthorized},
		{http.MethodPost, "/api/recipes", http.StatusUnauthorized},
		{http.MethodGet, "/api/recipes/1", http.StatusUnauthorized},
		{http.MethodPut, "/api/recipes/1", http.StatusUnauthorized},
		{http.MethodDelete, "/api/recipes/1", http.StatusUnauthorized},
		{http.MethodPatch, "/api/recipes/1", http.StatusNotFound},
		{http.MethodGet, "/recipe-categories", http.StatusNotFound},
		{http.MethodGet, "/recipes", http.StatusNotFound},
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
		{http.MethodPost, "/api/shopping-lists/1/shares", http.StatusUnauthorized},
		{http.MethodPost, "/api/shopping-list-shares/import", http.StatusUnauthorized},
		{http.MethodPost, "/shopping-lists/1/shares", http.StatusNotFound},
		{http.MethodPost, "/shopping-list-shares/import", http.StatusNotFound},
		{http.MethodGet, "/api/shopping-lists/1/shares", http.StatusNotFound},
		{http.MethodGet, "/api/shopping-list-shares/import", http.StatusNotFound},
		{http.MethodGet, "/api/shopping-list-shares/1", http.StatusNotFound},
		{http.MethodGet, "/api/products", http.StatusNotFound},
		{http.MethodGet, "/products", http.StatusNotFound},
		{http.MethodGet, "/api/users", http.StatusNotFound},
		{http.MethodGet, "/users", http.StatusNotFound},
		{http.MethodGet, "/api/activate/test-token", http.StatusNotFound},
		{http.MethodGet, "/api/activation/test-token", http.StatusNotFound},
		{http.MethodPost, "/api/activation", http.StatusBadRequest},
		{http.MethodPost, "/api/activation/resend", http.StatusBadRequest},
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

func TestRecipeMiddlewarePrivacyAndNoStore(t *testing.T) {
	var logs bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&logs, nil))
	engine := New(
		log, shopping.NewHandler(nil, log), recipe.NewHandler(nil, log), auth.NewAuthHandler(log, nil, false),
		middleware.NewAuthMiddleware(sharingTokenValidator{}),
		config.Config{AllowedOrigins: []string{"http://client.example.test"}},
	)
	for _, route := range []struct{ method, path string }{
		{"GET", "/api/recipe-categories"}, {"GET", "/api/recipes"}, {"POST", "/api/recipes"},
		{"GET", "/api/recipes/1"}, {"PUT", "/api/recipes/1"}, {"DELETE", "/api/recipes/1"},
	} {
		for _, tc := range []struct {
			name, authorization string
			origins             []string
			status              int
			code                string
		}{
			{"guest", "", nil, 401, "unauthorized"},
			{"bad access", "Bearer invalid-access-secret", nil, 401, "unauthorized"},
			{"foreign origin", "Bearer valid-access-secret", []string{"https://foreign.example.test"}, 403, "origin_forbidden"},
			{"null origin", "Bearer valid-access-secret", []string{"null"}, 403, "origin_forbidden"},
			{"duplicate origin", "Bearer valid-access-secret", []string{"http://client.example.test", "http://client.example.test"}, 403, "origin_forbidden"},
		} {
			t.Run(route.method+route.path+tc.name, func(t *testing.T) {
				req := httptest.NewRequest(route.method, route.path+"?private=query-secret", strings.NewReader(`{"title":"body-secret"`))
				req.Header.Set("Authorization", tc.authorization)
				req.Header.Set("Cookie", "refreshToken=cookie-secret")
				req.Header.Set("Referer", "http://client.example.test/recipes?private=referer-secret")
				for _, origin := range tc.origins {
					req.Header.Add("Origin", origin)
				}
				w := httptest.NewRecorder()
				engine.ServeHTTP(w, req)
				if w.Code != tc.status || !strings.Contains(w.Body.String(), `"code":"`+tc.code+`"`) ||
					w.Header().Get("Cache-Control") != "no-store" || len(w.Header().Values("Set-Cookie")) != 0 {
					t.Fatalf("Неверный отказ middleware: %d %s %v", w.Code, w.Body.String(), w.Header())
				}
			})
		}
		preflight := httptest.NewRequest("OPTIONS", route.path, nil)
		preflight.Header.Set("Origin", "http://client.example.test")
		preflight.Header.Set("Access-Control-Request-Method", route.method)
		preflight.Header.Set("Access-Control-Request-Headers", "Authorization,Content-Type,Idempotency-Key")
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, preflight)
		if w.Code != 204 || w.Header().Get("Cache-Control") != "no-store" ||
			w.Header().Get("Access-Control-Allow-Origin") != "http://client.example.test" {
			t.Fatal("Preflight рецептов отклонён или кеширует персональный ответ")
		}
	}
	for _, secret := range []string{"query-secret", "body-secret", "referer-secret", "cookie-secret", "access-secret"} {
		if strings.Contains(logs.String(), secret) {
			t.Fatal("HTTP-журнал раскрывает данные запроса")
		}
	}
}

type sharingTokenValidator struct{}

func (sharingTokenValidator) ValidateAccessToken(token string) (*auth.TokenClaims, error) {
	if token != "valid-access-secret" {
		return nil, errors.New("invalid token")
	}
	return &auth.TokenClaims{UserID: 7}, nil
}

func TestSharingMiddlewarePrivacyAndNoStore(t *testing.T) {
	var logs bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&logs, nil))
	engine := New(
		log, shopping.NewHandler(nil, log), recipe.NewHandler(nil, log), auth.NewAuthHandler(log, nil, false),
		middleware.NewAuthMiddleware(sharingTokenValidator{}),
		config.Config{AllowedOrigins: []string{"http://client.example.test"}},
	)
	for _, path := range []string{"/api/shopping-lists/1/shares", "/api/shopping-list-shares/import"} {
		for _, tc := range []struct {
			name, authorization string
			origins             []string
			status              int
			code                string
		}{
			{"guest", "", nil, 401, "unauthorized"},
			{"bad access", "Bearer invalid-access-secret", nil, 401, "unauthorized"},
			{"foreign origin", "Bearer valid-access-secret", []string{"https://foreign.example.test"}, 403, "origin_forbidden"},
			{"null origin", "Bearer valid-access-secret", []string{"null"}, 403, "origin_forbidden"},
			{"duplicate origin", "Bearer valid-access-secret",
				[]string{"http://client.example.test", "http://client.example.test"}, 403, "origin_forbidden"},
			{"no origin", "Bearer valid-access-secret", nil, 400, "invalid_request"},
			{"allowed origin", "Bearer valid-access-secret", []string{"http://client.example.test"}, 400, "invalid_request"},
		} {
			t.Run(path+tc.name, func(t *testing.T) {
				// Syntactically invalid JSON reaches no service; secrets must stay out of logs.
				req := httptest.NewRequest("POST", path+"?private=query-secret", strings.NewReader(`{"token":"body-secret"`))
				req.Header.Set("Authorization", tc.authorization)
				req.Header.Set("Cookie", "refreshToken=cookie-secret")
				req.Header.Set("Referer", "http://client.example.test/share#token=referer-secret")
				for _, origin := range tc.origins {
					req.Header.Add("Origin", origin)
				}
				w := httptest.NewRecorder()
				engine.ServeHTTP(w, req)
				if w.Code != tc.status || !strings.Contains(w.Body.String(), `"code":"`+tc.code+`"`) {
					t.Fatalf("Неверный отказ middleware: %d %s", w.Code, w.Body.String())
				}
				if w.Header().Get("Cache-Control") != "no-store" || len(w.Header().Values("Set-Cookie")) != 0 {
					t.Fatal("Отказ допускает кеширование или меняет cookie")
				}
			})
		}
		preflight := httptest.NewRequest("OPTIONS", path, nil)
		preflight.Header.Set("Origin", "http://client.example.test")
		preflight.Header.Set("Access-Control-Request-Method", "POST")
		preflight.Header.Set("Access-Control-Request-Headers", "Authorization,Content-Type,Idempotency-Key")
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, preflight)
		if w.Code != 204 || w.Header().Get("Access-Control-Allow-Origin") != "http://client.example.test" {
			t.Fatal("Preflight обмена отклонён")
		}
	}
	for _, secret := range []string{"query-secret", "body-secret", "referer-secret", "cookie-secret", "access-secret"} {
		if strings.Contains(logs.String(), secret) {
			t.Fatal("HTTP-журнал раскрывает секреты запроса")
		}
	}
	if !strings.Contains(logs.String(), "/api/shopping-lists/:listId/shares") {
		t.Fatal("Журнал должен сохранять безопасный шаблон маршрута")
	}
}

func TestCORSAllowedAndRejectedOrigins(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	engine := New(
		log, shopping.NewHandler(nil, log), recipe.NewHandler(nil, log),
		auth.NewAuthHandler(log, nil, false), middleware.NewAuthMiddleware(nil),
		config.Config{
			AllowedOrigins: []string{"http://client.example.test"},
			AuthLoginLimit: 1000, AuthRegistrationLimit: 1000, AuthResendLimit: 1000,
		},
	)
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
