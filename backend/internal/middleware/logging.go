package middleware

import (
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"
)


func RequestLogger(log *slog.Logger) gin.HandlerFunc {
	log = log.With("component", "http")

	return func(c *gin.Context) {
		
		start := time.Now()

		c.Next()

		logAttrs := []any{
			"method", c.Request.Method,
			"path", c.FullPath(),
			"status", c.Writer.Status(),
			"duration", time.Since(start),
			"client_ip", c.ClientIP(),
		}

		if len(c.Errors) > 0 {
			logAttrs = append(logAttrs, "errors", c.Errors.String())
		}

		switch  {
		case c.Writer.Status() >= 500:
			log.Error("request completed", logAttrs...)

		case c.Writer.Status() >= 400:
			log.Warn("request completed", logAttrs...)

		default:
			log.Info("request completed", logAttrs...)
		}
	}
}