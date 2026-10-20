package auth

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/gin-gonic/gin"
	"math"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAuthFieldValidation(t *testing.T) {
	for _, tc := range []struct {
		name, email, password string
		valid                 bool
	}{
		{"Имя", "user@example.test", "пароль", true},
		{strings.Repeat("😀", 20), "USER@example.test", strings.Repeat("я", 36), true},
		{"А", "user@example.test", "password", false},
		{strings.Repeat("😀", 21), "user@example.test", "password", false},
		{"Им\nя", "user@example.test", "password", false},
		{"\t ", "user@example.test", "password", false},
		{"Имя", "Name <user@example.test>", "password", false},
		{"Имя", "пользователь@example.test", "password", false},
		{"Имя", "user@localhost", "password", false},
		{"Имя", strings.Repeat("a", 65) + "@example.test", "password", false},
		{"Имя", "user@" + strings.Repeat("a", 245) + ".test", "password", false},
		{"Имя", "user@example.test", strings.Repeat("я", 37), false},
		{"Имя", "user@example.test", "12345", false},
	} {
		req := RegistrationRequest{Name: tc.name, Email: tc.email, Password: tc.password}
		err := validateAuthFields(&req)
		if (err == nil) != tc.valid {
			t.Fatalf("validation: valid=%v error=%v", tc.valid, err)
		}
	}
	req := RegistrationRequest{Name: "\u0085 Имя \u00a0", Email: "\u0085 USER@Example.Test \u00a0", Password: "password123"}
	if err := validateAuthFields(&req); err != nil || req.Name != "Имя" || req.Email != "user@example.test" {
		t.Fatal("Normalization mismatch")
	}
	login := LoginRequest{Email: "user@example.test", Password: "short"}
	if err := validateAuthFields(&login); err != nil {
		t.Fatal("Login must not enforce new minimum password length")
	}
}

func TestAuthJSONLimitsAndNormalization(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, endpoint := range []string{"/registration", "/login", "/activation", "/activation/resend"} {
		fake := &authFake{}
		handler := NewAuthHandler(testLog(), fake, false)
		engine := gin.New()
		engine.POST("/registration", handler.Registration)
		engine.POST("/login", handler.Login)
		engine.POST("/activation", handler.Activate)
		engine.POST("/activation/resend", handler.ResendActivation)
		body := `{"email":"` + strings.Repeat("x", 4096) + `"}`
		response := httptest.NewRecorder()
		request := httptest.NewRequest("POST", endpoint, strings.NewReader(body))
		// Unknown-length streaming body must be limited too.
		request.ContentLength = -1
		engine.ServeHTTP(response, request)
		if response.Code != 413 || fake.calls != 0 {
			t.Fatal("Oversized request reached service")
		}
	}
	fake := &authFake{}
	handler := NewAuthHandler(testLog(), fake, false)
	engine := gin.New()
	engine.POST("/registration", handler.Registration)
	for _, body := range []string{`{"name":"x","email":"invalid","password":"short"}`, `{"name":"Имя","email":"user@example.test","password":"` + strings.Repeat("я", 37) + `"}`} {
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, httptest.NewRequest("POST", "/registration", strings.NewReader(body)))
		if response.Code != 400 || !strings.Contains(response.Body.String(), "fieldErrors") || fake.calls != 0 {
			t.Fatal("Invalid fields must return structured 400 before service")
		}
	}
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, httptest.NewRequest("POST", "/registration", strings.NewReader(`{"name":"  Имя  ","email":" USER@Example.Test ","password":"password123"}`)))
	if response.Code != 201 || fake.name != "Имя" || fake.email != "user@example.test" {
		t.Fatal("Handler did not normalize")
	}
}

func TestServiceValidatesBeforeHashAndRepository(t *testing.T) {
	users, tokens, mail := &usersFake{}, &tokensFake{}, &mailFake{}
	service := newTestUserService(testLog(), users, tokens, mail, "")
	_, err := service.Registration(context.Background(), "Имя", "user@example.test", strings.Repeat("я", 37))
	var invalid *ValidationError
	if !errors.As(err, &invalid) || users.created.Name != "" || mail.calls != 0 {
		t.Fatal("Validation must precede hashing and persistence")
	}
	_, err = service.Registration(context.Background(), "  Имя  ", " USER@Example.Test ", "password123")
	if err != nil || users.created.Name != "Имя" || users.created.Email != "user@example.test" {
		t.Fatal("Service normalization mismatch")
	}
}

func TestUserDTOStringBIGINT(t *testing.T) {
	data, err := json.Marshal(NewUserDTO(User{ID: math.MaxInt64}))
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err := json.Unmarshal(data, &fields); err != nil || fields["id"] != "9223372036854775807" {
		t.Fatal("User ID must remain an exact decimal string")
	}
}
