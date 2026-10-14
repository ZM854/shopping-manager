package router

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/ZM854/shopping-manager/backend/internal/auth"
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
	frontendURL string,
) *gin.Engine {
	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(middleware.RequestLogger(log))

	router.Use(cors.New(cors.Config{
		AllowOrigins: []string{
			frontendURL,
			"http://localhost:5173",
			"http://localhost:4173",
		},
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
		MaxAge:           12 * time.Hour,
	}))

	api := router.Group("/api")
	api.GET("/healthz", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})
	api.POST("/registration", authHandler.Registration)
	api.POST("/login", authHandler.Login)
	api.POST("/logout", authHandler.Logout)
	api.GET("/activate/:link", authHandler.Activate)
	api.POST("/refresh", authHandler.Refresh)
	api.GET("/users", authHandler.GetUsers)

	shoppingAPI := api.Group("")
	shoppingAPI.Use(authMiddleware.HandleAuth())
	shoppingHandler.Register(shoppingAPI)

	return router
}
