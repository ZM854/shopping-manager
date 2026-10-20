package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Cookie-changing endpoints require Origin even for non-browser clients.
func CheckOrigin(origins []string) gin.HandlerFunc {
	allowed := make(map[string]bool, len(origins))
	for _, origin := range origins {
		allowed[origin] = true
	}
	return func(c *gin.Context) {
		values := c.Request.Header.Values("Origin")
		origin := c.GetHeader("Origin")
		requiresOrigin := c.Request.Method == http.MethodPost &&
			(c.Request.URL.Path == "/api/refresh" || c.Request.URL.Path == "/api/logout")
		if (len(values) > 0 && (len(values) != 1 || !allowed[origin])) || (requiresOrigin && origin == "") {
			c.Header("Cache-Control", "no-store")
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"code": "origin_forbidden", "message": "Источник запроса не разрешён",
			})
			return
		}
		c.Next()
	}
}
