package middleware

import (
	"errors"
	"net/http"
	"strings"

	"github.com/ZM854/shopping-manager/backend/internal/auth"
	"github.com/gin-gonic/gin"
)

const (
	authorizationHeader = "Authorization"
	bearerPrefix        = "Bearer "
)

type TokenValidator interface {
	ValidateAccessToken(tokenString string) (*auth.TokenClaims, error)
}

type AuthMiddleware struct {
	validator TokenValidator
}

func NewAuthMiddleware(validator TokenValidator) *AuthMiddleware {
	return &AuthMiddleware{
		validator: validator,
	}
}

func (m *AuthMiddleware) HandleAuth() gin.HandlerFunc { return m.handleAuth(false) }

func (m *AuthMiddleware) HandleShoppingAuth() gin.HandlerFunc { return m.handleAuth(true) }

func (m *AuthMiddleware) handleAuth(shopping bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		deny := func(message string) {
			if shopping {
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"code": "unauthorized", "message": "Требуется авторизация"})
			} else {
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": message})
			}
		}
		header := c.GetHeader(authorizationHeader)

		if header == "" {
			deny("authorization header is missing")
			return
		}

		if !strings.HasPrefix(header, bearerPrefix) {
			deny("invalid authorization header")
			return
		}

		token := strings.TrimPrefix(header, bearerPrefix)

		claims, err := m.validator.ValidateAccessToken(token)

		if err != nil {
			if errors.Is(err, auth.ErrInvalidToken) {
				deny("invalid access token")
				return
			}

			deny("failed to validate access token")
			return
		}
		if claims == nil || claims.UserID <= 0 {
			deny("invalid access token")
			return
		}

		c.Set("userID", claims.UserID)

		c.Next()
	}
}
