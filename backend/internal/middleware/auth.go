package middleware

import (
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

func (m *AuthMiddleware) HandleAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		deny := func() {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"code":    "unauthorized",
				"message": "Требуется авторизация",
			})
		}
		header := c.GetHeader(authorizationHeader)
		if !strings.HasPrefix(header, bearerPrefix) {
			deny()
			return
		}
		token := strings.TrimPrefix(header, bearerPrefix)
		claims, err := m.validator.ValidateAccessToken(token)
		if err != nil || claims == nil || claims.UserID <= 0 {
			deny()
			return
		}
		c.Set("userID", claims.UserID)
		c.Next()
	}
}
