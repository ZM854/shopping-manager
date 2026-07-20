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
	bearerPrefix = "Bearer "
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

func (m *AuthMiddleware) HandleAuth() gin.HandlerFunc  {
	return func(c *gin.Context) {
		header := c.GetHeader(authorizationHeader)

		if header == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "authorization header is missing",
			})
			return
		}

		if !strings.HasPrefix(header, bearerPrefix) {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "invalid authorization header",
			})
			return
		}

		token := strings.TrimPrefix(header, bearerPrefix)

		claims, err := m.validator.ValidateAccessToken(token)

		if err != nil {
			if errors.Is(err, auth.ErrInvalidToken) {
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
					"error": "invalid access token",
				})
				return
			}

			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "failed to validate access token",
			})
			return
		}

		c.Set("userID", claims.UserID)

		c.Next()
	}
}