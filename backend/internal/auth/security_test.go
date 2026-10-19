package auth

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestJWTRequiresAlgorithmPurposeExpiryAndUser(t *testing.T) {
	const key = "test-only-shared-key-for-purpose-validation"
	service := NewTokenService(testLog(), nil, key, key, time.Minute, time.Hour)
	for _, purpose := range []string{"access", "refresh"} {
		for _, scenario := range []string{
			"valid", "HS384", "HS512", "missing_exp", "expired", "missing_sub",
			"wrong_sub", "missing_user", "zero_user", "negative_user", "wrong_key",
		} {
			t.Run(purpose+"/"+scenario, func(t *testing.T) {
				claims := jwt.MapClaims{
					"user_id": 42,
					"sub":     purpose,
					"exp":     time.Now().Add(time.Minute).Unix(),
				}
				var method jwt.SigningMethod = jwt.SigningMethodHS256
				signingKey := key
				switch scenario {
				case "HS384":
					method = jwt.SigningMethodHS384
				case "HS512":
					method = jwt.SigningMethodHS512
				case "missing_exp":
					delete(claims, "exp")
				case "expired":
					claims["exp"] = time.Now().Add(-time.Minute).Unix()
				case "missing_sub":
					delete(claims, "sub")
				case "wrong_sub":
					claims["sub"] = "refresh"
					if purpose == "refresh" {
						claims["sub"] = "access"
					}
				case "missing_user":
					delete(claims, "user_id")
				case "zero_user":
					claims["user_id"] = 0
				case "negative_user":
					claims["user_id"] = -1
				case "wrong_key":
					signingKey = "different-test-only-key"
				}
				token, err := jwt.NewWithClaims(method, claims).SignedString([]byte(signingKey))
				if err != nil {
					t.Fatal(err)
				}
				validate := service.ValidateAccessToken
				if purpose == "refresh" {
					validate = service.ValidateRefreshToken
				}
				result, err := validate(token)
				if scenario == "valid" {
					if err != nil || result.UserID != 42 {
						t.Fatal("Valid token rejected")
					}
				} else if result != nil || !errors.Is(err, ErrInvalidToken) {
					t.Fatal("Invalid token must return ErrInvalidToken without claims")
				}
			})
		}
	}
	for _, id := range []int64{0, -1} {
		if _, err := service.GenerateTokens(id); !errors.Is(err, ErrInvalidToken) {
			t.Fatal("Cannot issue a token for a nonpositive user ID")
		}
	}
}

func TestInternalSecretsCannotBeSerialized(t *testing.T) {
	for _, model := range []any{
		User{ID: 42, PasswordHash: "private-password-hash", ActivationToken: "private-activation"},
		CreateUserRequest{PasswordHash: "private-password-hash", ActivationToken: "private-activation"},
		UpdateUserRequest{PasswordHash: "private-password-hash", ActivationToken: "private-activation"},
		RefreshToken{TokenHash: "private-refresh-hash"},
		TokenPair{AccessToken: "private-access", RefreshToken: "private-refresh"},
		AuthResult{Tokens: TokenPair{AccessToken: "private-access", RefreshToken: "private-refresh"}},
	} {
		data, err := json.Marshal(model)
		if err != nil {
			t.Fatal(err)
		}
		for _, secret := range []string{"private-", "PasswordHash", "ActivationToken", "TokenHash", "RefreshToken"} {
			if strings.Contains(string(data), secret) {
				t.Fatalf("Internal secret serialized from %T", model)
			}
		}
	}
}
