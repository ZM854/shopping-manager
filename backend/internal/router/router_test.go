package router

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ZM854/shopping-manager/backend/internal/auth"
	"github.com/ZM854/shopping-manager/backend/internal/config"
	"github.com/ZM854/shopping-manager/backend/internal/middleware"
	"github.com/ZM854/shopping-manager/backend/internal/shopping"
)

func TestRateLimitAndProxyTrust(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	for _, trusted := range []bool{false, true} {
		cfg := config.Config{AllowedOrigins: []string{"http://client.example.test"}, AuthLoginLimit: 2, AuthRegistrationLimit: 2, AuthResendLimit: 2}
		if trusted {
			cfg.TrustedProxies = []string{"127.0.0.1"}
		}
		engine := New(log, shopping.NewHandler(nil, log), auth.NewAuthHandler(log, nil, false), middleware.NewAuthMiddleware(nil), cfg)
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

func TestCORSAllowedAndRejectedOrigins(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	engine := New(log, shopping.NewHandler(nil, log), auth.NewAuthHandler(log, nil, false), middleware.NewAuthMiddleware(nil), config.Config{AllowedOrigins: []string{"http://client.example.test"}, AuthLoginLimit: 1000, AuthRegistrationLimit: 1000, AuthResendLimit: 1000})
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
