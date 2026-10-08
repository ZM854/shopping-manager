package auth

import (
	"context"
	"errors"
	"fmt"
	"github.com/gin-gonic/gin"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type authFake struct {
	err                          error
	calls                        int
	ctx                          context.Context
	token, email, name, password string
}

func (f *authFake) response() (AuthResponse, error) {
	f.calls++
	return AuthResponse{User: UserDTO{ID: 42, Name: "Имя"}, TokenPair: TokenPair{AccesToken: "test-access", RefreshToken: "test-refresh"}}, f.err
}
func (f *authFake) Registration(ctx context.Context, name, email, password string) (AuthResponse, error) {
	f.ctx, f.name, f.email, f.password = ctx, name, email, password
	return f.response()
}
func (f *authFake) Login(ctx context.Context, email, password string) (AuthResponse, error) {
	f.ctx, f.email, f.password = ctx, email, password
	return f.response()
}
func (f *authFake) Refresh(ctx context.Context, token string) (AuthResponse, error) {
	f.ctx, f.token = ctx, token
	return f.response()
}
func (f *authFake) Logout(ctx context.Context, token string) error {
	f.ctx, f.token = ctx, token
	f.calls++
	return f.err
}
func (f *authFake) Activate(ctx context.Context, token string) error {
	f.ctx, f.token = ctx, token
	f.calls++
	return f.err
}
func (f *authFake) GetAllUsers(ctx context.Context) ([]User, error) {
	f.ctx = ctx
	f.calls++
	return nil, f.err
}
func TestAuthHTTPResponses(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ops := []struct {
		method, path   string
		fn             func(*AuthHandler) gin.HandlerFunc
		success        int
		cookie         bool
		business       error
		businessStatus int
	}{
		{"POST", "/registration", func(h *AuthHandler) gin.HandlerFunc { return h.Registration }, 201, false, ErrUserAlreadyExist, 409},
		{"POST", "/login", func(h *AuthHandler) gin.HandlerFunc { return h.Login }, 200, false, ErrInvalidCredentials, 401},
		{"POST", "/refresh", func(h *AuthHandler) gin.HandlerFunc { return h.Refresh }, 200, true, ErrInvalidToken, 401},
		{"POST", "/logout", func(h *AuthHandler) gin.HandlerFunc { return h.Logout }, 200, true, nil, 500},
		{"GET", "/activate/:link", func(h *AuthHandler) gin.HandlerFunc { return h.Activate }, 302, false, ErrInvalidActivation, 400},
	}
	for _, op := range ops {
		t.Run(op.path, func(t *testing.T) {
			for _, scenario := range []string{"success", "failure", "business", "missing_cookie", "invalid_body", "inactive"} {
				if scenario == "business" && op.business == nil || scenario == "missing_cookie" && !op.cookie || scenario == "invalid_body" && op.path != "/login" && op.path != "/registration" || scenario == "inactive" && op.path != "/login" {
					continue
				}
				t.Run(scenario, func(t *testing.T) {
					f := &authFake{}
					want := op.success
					switch scenario {
					case "failure":
						f.err = errors.New("private database error")
						want = 500
						if op.path == "/refresh" {
							want = 401
						}
					case "business":
						f.err = fmt.Errorf("wrapped: %w", op.business)
						want = op.businessStatus
					case "missing_cookie":
						want = 401
					case "invalid_body":
						want = 400
					case "inactive":
						f.err = ErrUserNotActivated
						want = 403
					}
					h := NewAuthHandler(testLog(), f, "https://client.example.test/login")
					r := gin.New()
					r.Handle(op.method, op.path, op.fn(h))
					body := `{"name":"Имя","email":"user@example.test","password":"password123"}`
					if scenario == "invalid_body" {
						body = `{"email":"invalid","password":"x"}`
					}
					req := httptest.NewRequest(op.method, strings.ReplaceAll(op.path, ":link", "activation"), strings.NewReader(body))
					if op.cookie && scenario != "missing_cookie" {
						req.AddCookie(&http.Cookie{Name: refreshCookieName, Value: "incoming-refresh"})
					}
					w := httptest.NewRecorder()
					r.ServeHTTP(w, req)
					if w.Code != want {
						t.Fatalf("status=%d want=%d: %s", w.Code, want, w.Body.String())
					}
					if scenario == "invalid_body" || scenario == "missing_cookie" {
						if f.calls != 0 {
							t.Fatal("Invalid input reached service")
						}
						return
					}
					if f.calls != 1 || f.ctx != req.Context() {
						t.Fatal("Request context was lost")
					}
					if op.cookie && f.token != "incoming-refresh" {
						t.Fatal("Wrong refresh cookie forwarded")
					}
					if scenario == "success" {
						switch op.path {
						case "/registration", "/login", "/refresh":
							cookies := w.Result().Cookies()
							if len(cookies) != 1 || cookies[0].Value != "test-refresh" || !cookies[0].HttpOnly || cookies[0].Path != "/" {
								t.Fatal("Refresh cookie missing")
							}
						case "/logout":
							cookies := w.Result().Cookies()
							if len(cookies) != 1 || cookies[0].MaxAge >= 0 {
								t.Fatal("Logout must clear cookie")
							}
						case "/activate/:link":
							if w.Header().Get("Location") != "https://client.example.test/login" || f.token != "activation" {
								t.Fatal("Wrong activation redirect")
							}
						}
					}
					if strings.Contains(w.Body.String(), "private database error") {
						t.Fatal("Internal error leaked")
					}
				})
			}
		})
	}
	// Не закрепляем выдачу внутренних User как корректный публичный контракт.
	f := &authFake{err: errors.New("database failure")}
	r := gin.New()
	r.GET("/users", NewAuthHandler(testLog(), f, "").GetUsers)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/users", nil))
	if w.Code != 500 {
		t.Fatal("User query error must be handled")
	}
}
