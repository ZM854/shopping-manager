//go:build integration

package integration_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"os"

	"github.com/ZM854/shopping-manager/backend/internal/auth"
	"github.com/ZM854/shopping-manager/backend/internal/config"
	"github.com/ZM854/shopping-manager/backend/internal/database"
	"github.com/ZM854/shopping-manager/backend/internal/middleware"
	"github.com/ZM854/shopping-manager/backend/internal/product"
	"github.com/ZM854/shopping-manager/backend/internal/router"
	"github.com/ZM854/shopping-manager/backend/internal/testutil/testdb"
	"github.com/jackc/pgx/v5/pgxpool"
)

func repoFixture(t *testing.T) (*pgxpool.Pool, *auth.UserRepository, *product.ProductRepository, *auth.TokenRepository, *slog.Logger) {
	t.Helper()
	pool := testdb.New(t)
	testdb.ApplyMigrations(t, pool, 0, 3)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	return pool, auth.NewUserRepository(pool, log), product.NewProductRepository(pool, log), auth.NewTokenRepository(pool, log), log
}
func createUser(t *testing.T, repo *auth.UserRepository, n int) auth.User {
	t.Helper()
	user, err := repo.Create(context.Background(), auth.CreateUserRequest{Name: fmt.Sprintf("User %d", n), Email: fmt.Sprintf("user%d@example.test", n), PasswordHash: "test-hash", ActivationToken: fmt.Sprintf("activation-%d", n)})
	if err != nil {
		t.Fatal(err)
	}
	return user
}
func TestProductRepositoryCRUDAndUserIsolation(t *testing.T) {
	t.Parallel()
	_, users, repo, _, _ := repoFixture(t)
	ctx := context.Background()
	first, second := createUser(t, users, 1), createUser(t, users, 2)
	items, err := repo.GetProducts(ctx, first.ID)
	if err != nil || items == nil || len(items) != 0 {
		t.Fatal("Empty list must be a non-nil slice")
	}
	a, err := repo.CreateProduct(ctx, first.ID, product.CreateProductRequest{Name: "Молоко", Quantity: 1.5, Unit: "л"})
	if err != nil {
		t.Fatal(err)
	}
	b, err := repo.CreateProduct(ctx, second.ID, product.CreateProductRequest{Name: "Хлеб", Quantity: 2, Unit: "шт"})
	if err != nil {
		t.Fatal(err)
	}
	last, err := repo.CreateProduct(ctx, first.ID, product.CreateProductRequest{Name: "Яйца", Quantity: 2, Unit: "шт"})
	if err != nil {
		t.Fatal(err)
	}
	ordered, err := repo.GetProducts(ctx, first.ID)
	if err != nil || len(ordered) != 2 || ordered[0].ID != a.ID || ordered[1].ID != last.ID {
		t.Fatal("Product order or isolation lost")
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := repo.CreateProduct(canceled, first.ID, product.CreateProductRequest{Name: "Canceled", Quantity: 1}); err == nil {
		t.Fatal("Canceled request created product")
	}
	ordered, err = repo.GetProducts(ctx, first.ID)
	if err != nil || len(ordered) != 2 {
		t.Fatal("Canceled request changed products")
	}
	loaded, err := repo.GetProduct(ctx, first.ID, a.ID)
	if err != nil || loaded.Name != "Молоко" || loaded.Quantity != 1.5 || loaded.Unit != "л" {
		t.Fatal("Product read lost fields")
	}
	if _, err := repo.GetProduct(ctx, second.ID, a.ID); !errors.Is(err, product.ErrProductNotFound) {
		t.Fatal("Foreign product readable")
	}
	update := product.UpdateProductRequest{Name: "Кефир", Quantity: 2.5, Unit: "л", IsMarked: true}
	if _, err := repo.UpdateProduct(ctx, second.ID, a.ID, update); !errors.Is(err, product.ErrProductNotFound) {
		t.Fatal("Foreign product editable")
	}
	if err := repo.DeleteProduct(ctx, second.ID, a.ID); !errors.Is(err, product.ErrProductNotFound) {
		t.Fatal("Foreign product deletable")
	}
	changed, err := repo.UpdateProduct(ctx, first.ID, a.ID, update)
	if err != nil || changed.Name != "Кефир" || !changed.IsMarked || changed.Quantity != 2.5 {
		t.Fatal("Product update failed")
	}
	if err := repo.DeleteAllProducts(ctx, first.ID); err != nil {
		t.Fatal(err)
	}
	items, err = repo.GetProducts(ctx, first.ID)
	if err != nil || len(items) != 0 {
		t.Fatal("Owner list not cleared")
	}
	other, err := repo.GetProducts(ctx, second.ID)
	if err != nil || len(other) != 1 || other[0].ID != b.ID {
		t.Fatal("Clear affected another user")
	}
	if err := repo.DeleteProduct(ctx, second.ID, b.ID); err != nil {
		t.Fatal(err)
	}
	if err := repo.DeleteProduct(ctx, second.ID, b.ID); !errors.Is(err, product.ErrProductNotFound) {
		t.Fatal("Deleted product should not exist")
	}
	if _, err := repo.UpdateProduct(ctx, first.ID, a.ID, update); !errors.Is(err, product.ErrProductNotFound) {
		t.Fatal("Deleted product updated")
	}
}
func TestUserAndTokenRepositories(t *testing.T) {
	t.Parallel()
	_, users, _, tokens, _ := repoFixture(t)
	ctx := context.Background()
	first, second := createUser(t, users, 1), createUser(t, users, 2)
	byID, err := users.GetById(ctx, first.ID)
	if err != nil || byID != first {
		t.Fatal("User ID query failed")
	}
	byEmail, err := users.GetByEmail(ctx, first.Email)
	if err != nil || byEmail != first {
		t.Fatal("Email query failed")
	}
	byToken, err := users.GetByActivationToken(ctx, first.ActivationToken)
	if err != nil || byToken != first {
		t.Fatal("Activation query failed")
	}
	all, err := users.GetAll(ctx)
	if err != nil || len(all) != 2 || all[0].ID != first.ID || all[1].ID != second.ID {
		t.Fatal("User ordering failed")
	}
	if _, err := users.Create(ctx, auth.CreateUserRequest{Name: "Duplicate", Email: first.Email, PasswordHash: "test", ActivationToken: "distinct-token"}); !errors.Is(err, auth.ErrUserAlreadyExist) {
		t.Fatal("Duplicate email accepted")
	}
	updated, err := users.Update(ctx, first.ID, auth.UpdateUserRequest{Name: "Updated", Email: first.Email, PasswordHash: first.PasswordHash, IsEmailVerified: true, ActivationToken: first.ActivationToken})
	if err != nil || updated.Name != "Updated" || !updated.IsEmailVerified {
		t.Fatal("User update failed")
	}
	for _, query := range []func() error{func() error { _, e := users.GetById(ctx, 9999); return e }, func() error { _, e := users.GetByEmail(ctx, "missing@example.test"); return e }, func() error { _, e := users.GetByActivationToken(ctx, "missing"); return e }, func() error { _, e := users.Update(ctx, 9999, auth.UpdateUserRequest{}); return e }} {
		if !errors.Is(query(), auth.ErrUserNotFound) {
			t.Fatal("Missing user must map to domain error")
		}
	}
	if _, err := tokens.FindByUserId(ctx, first.ID); !errors.Is(err, auth.ErrRefreshTokenNotFound) {
		t.Fatal("Absent session accepted")
	}
	if err := tokens.Save(ctx, first.ID, "hash-first"); err != nil {
		t.Fatal(err)
	}
	if err := tokens.Save(ctx, second.ID, "hash-second"); err != nil {
		t.Fatal(err)
	}
	if err := tokens.Save(ctx, first.ID, "hash-replaced"); err != nil {
		t.Fatal(err)
	}
	stored, err := tokens.FindByUserId(ctx, first.ID)
	if err != nil || stored.TokenHash != "hash-replaced" || stored.UserID != first.ID {
		t.Fatal("Token replacement failed")
	}
	if err := tokens.DeleteByUserId(ctx, first.ID); err != nil {
		t.Fatal(err)
	}
	if err := tokens.DeleteByUserId(ctx, first.ID); !errors.Is(err, auth.ErrRefreshTokenNotFound) {
		t.Fatal("Deleted session accepted")
	}
	stored, err = tokens.FindByUserId(ctx, second.ID)
	if err != nil || stored.TokenHash != "hash-second" {
		t.Fatal("Logout affected another user")
	}
}
func TestRepositoriesPropagateConnectionFailures(t *testing.T) {
	pool, users, items, tokens, _ := repoFixture(t)
	pool.Close()
	ctx := context.Background()
	operations := []func() error{
		func() error { _, e := items.GetProducts(ctx, 1); return e }, func() error { _, e := items.GetProduct(ctx, 1, 1); return e }, func() error {
			_, e := items.CreateProduct(ctx, 1, product.CreateProductRequest{Name: "test"})
			return e
		}, func() error { _, e := items.UpdateProduct(ctx, 1, 1, product.UpdateProductRequest{}); return e }, func() error { return items.DeleteProduct(ctx, 1, 1) }, func() error { return items.DeleteAllProducts(ctx, 1) },
		func() error { _, e := users.GetAll(ctx); return e }, func() error { _, e := users.GetById(ctx, 1); return e }, func() error { _, e := users.GetByEmail(ctx, "test@example.test"); return e }, func() error { _, e := users.GetByActivationToken(ctx, "test"); return e }, func() error { _, e := users.Create(ctx, auth.CreateUserRequest{}); return e }, func() error { _, e := users.Update(ctx, 1, auth.UpdateUserRequest{}); return e },
		func() error { return tokens.Save(ctx, 1, "hash") }, func() error { _, e := tokens.FindByUserId(ctx, 1); return e }, func() error { return tokens.DeleteByUserId(ctx, 1) },
	}
	for i, operation := range operations {
		if err := operation(); err == nil {
			t.Fatalf("Operation %d suppressed DB failure", i)
		}
	}
}

type activationCapture struct{ link string }

func (m *activationCapture) SendActivationMail(_ context.Context, _ string, link string) error {
	m.link = link
	return nil
}
func TestAuthHTTPWithPostgres(t *testing.T) {
	_, users, items, tokens, log := repoFixture(t)
	tokenService := auth.NewTokenService(log, tokens, "test-access-secret", "test-refresh-secret", time.Minute, time.Hour)
	mail := &activationCapture{}
	engine := router.New(log, product.NewProductHandler(product.NewProductService(items, log), log), auth.NewAuthHandler(log, auth.NewUserService(log, users, tokenService, mail, "http://example.test/api/activate"), "http://client.example.test/login"), middleware.NewAuthMiddleware(tokenService), "http://client.example.test")
	request := func(method, path, body, bearer string, cookie *http.Cookie) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		if bearer != "" {
			req.Header.Set("Authorization", "Bearer "+bearer)
		}
		if cookie != nil {
			req.AddCookie(cookie)
		}
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)
		return w
	}
	registered := request("POST", "/api/registration", `{"name":"Имя","email":"flow@example.test","password":"password123"}`, "", nil)
	if registered.Code != 201 || mail.link == "" {
		t.Fatalf("Registration failed: %s", registered.Body.String())
	}
	loginBody := `{"email":"flow@example.test","password":"password123"}`
	if w := request("POST", "/api/login", loginBody, "", nil); w.Code != 403 {
		t.Fatal("Inactive login must be rejected")
	}
	activationPath := strings.TrimPrefix(mail.link, "http://example.test")
	if w := request("GET", activationPath, "", "", nil); w.Code != 302 {
		t.Fatal("Activation failed")
	}
	login := request("POST", "/api/login", loginBody, "", nil)
	if login.Code != 200 {
		t.Fatalf("Login failed: %s", login.Body.String())
	}
	var session auth.AuthResponse
	if err := json.Unmarshal(login.Body.Bytes(), &session); err != nil {
		t.Fatal(err)
	}
	cookie := login.Result().Cookies()[0]
	created := request("POST", "/api/products", `{"name":"Молоко","quantity":1.5,"unit":"л"}`, session.AccesToken, nil)
	if created.Code != 201 {
		t.Fatalf("Product failed: %s", created.Body.String())
	}
	var item product.Product
	if err := json.Unmarshal(created.Body.Bytes(), &item); err != nil {
		t.Fatal(err)
	}
	second := createUser(t, users, 2)
	secondTokens, err := tokenService.GenerateTokens(second.ID)
	if err != nil {
		t.Fatal(err)
	}
	path := fmt.Sprintf("/api/products/%d", item.ID)
	for _, method := range []string{"GET", "PUT", "DELETE"} {
		w := request(method, path, `{"name":"foreign","quantity":1}`, secondTokens.AccesToken, nil)
		if w.Code != 404 {
			t.Fatalf("Foreign %s returned %d", method, w.Code)
		}
	}
	if w := request("GET", path, "", session.AccesToken, nil); w.Code != 200 {
		t.Fatal("Foreign operations changed owner's item")
	}
	refresh := request("POST", "/api/refresh", "", "", cookie)
	if refresh.Code != 200 {
		t.Fatalf("Refresh failed: %s", refresh.Body.String())
	}
	cookie = refresh.Result().Cookies()[0]
	if w := request("POST", "/api/logout", "", "", cookie); w.Code != 200 {
		t.Fatal("Logout failed")
	}
	if w := request("POST", "/api/refresh", "", "", cookie); w.Code != 401 {
		t.Fatal("Revoked refresh accepted")
	}
}
func TestPostgresConnectionFactory(t *testing.T) {
	cfg, err := testdb.ParseConfig(os.Getenv("TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	conn := cfg.ConnConfig
	pool, err := database.NewPostgres(config.Config{DBHost: conn.Host, DBPort: fmt.Sprint(conn.Port), DBUser: conn.User, DBPassword: conn.Password, DBName: conn.Database, DBSSLMode: "disable"}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if pool.Config().MaxConns != 10 || pool.Config().MinConns != 2 || pool.Config().MaxConnLifetime != time.Hour {
		t.Fatal("Incorrect pool settings")
	}
	var value int
	if err := pool.QueryRow(context.Background(), "SELECT 1").Scan(&value); err != nil || value != 1 {
		t.Fatal("Connection not usable")
	}
}
