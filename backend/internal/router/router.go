package router

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/ZM854/shopping-manager/backend/internal/auth"
	"github.com/ZM854/shopping-manager/backend/internal/config"
	"github.com/ZM854/shopping-manager/backend/internal/middleware"
	"github.com/ZM854/shopping-manager/backend/internal/shopping"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

func New(
	log *slog.Logger,
	shoppingHandler *shopping.Handler,
	authHandler *auth.AuthHandler,
	authMiddleware *middleware.AuthMiddleware,
	cfg config.Config,
) *gin.Engine {
	router := gin.New()
	if err := router.SetTrustedProxies(cfg.TrustedProxies); err != nil {
		panic("invalid trusted proxy configuration")
	}
	router.Use(gin.Recovery())
	router.Use(middleware.RequestLogger(log))
	router.Use(middleware.CheckOrigin(cfg.AllowedOrigins))

	router.Use(cors.New(cors.Config{
		AllowOrigins: cfg.AllowedOrigins,
		AllowMethods: []string{
			"GET",
			"POST",
			"PUT",
			"PATCH",
			"DELETE",
			"OPTIONS",
		},
		AllowHeaders: []string{
			"Origin",
			"Content-Type",
			"Accept",
			"Authorization",
			"Idempotency-Key",
		},
		AllowCredentials: true,
		ExposeHeaders:    []string{"Retry-After"},
		MaxAge:           12 * time.Hour,
	}))

	api := router.Group("/api")
	api.Use(middleware.NewRateLimiter(map[string]middleware.RateRule{
		"/api/login":             {Limit: cfg.AuthLoginLimit, Window: time.Minute},
		"/api/registration":      {Limit: cfg.AuthRegistrationLimit, Window: time.Hour},
		"/api/activation/resend": {Limit: cfg.AuthResendLimit, Window: 10 * time.Minute},
	}).Handle())
	api.GET("/healthz", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})
	api.POST("/registration", authHandler.Registration)
	api.POST("/login", authHandler.Login)
	api.POST("/logout", authHandler.Logout)
	api.POST("/activation", authHandler.Activate)
	api.POST("/activation/resend", authHandler.ResendActivation)
	api.POST("/refresh", authHandler.Refresh)

	shoppingAPI := api.Group("")
	shoppingAPI.Use(authMiddleware.HandleAuth())
	shoppingHandler.Register(shoppingAPI)

	return router
}
