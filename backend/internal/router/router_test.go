package router

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ZM854/shopping-manager/backend/internal/auth"
	"github.com/ZM854/shopping-manager/backend/internal/middleware"
	"github.com/ZM854/shopping-manager/backend/internal/product"
)

func TestAPIPrefixAndAuthorization(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	engine := New(
		log,
		product.NewProductHandler(nil, log),
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
		{http.MethodGet, "/api/products", http.StatusUnauthorized},
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
