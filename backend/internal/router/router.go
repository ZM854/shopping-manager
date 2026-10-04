package router

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/ZM854/shopping-manager/backend/internal/auth"
	"github.com/ZM854/shopping-manager/backend/internal/middleware"
	"github.com/ZM854/shopping-manager/backend/internal/product"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

func New(
	log *slog.Logger,
	productHandler *product.ProductHandler,
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

	api.Use(authMiddleware.HandleAuth())

	api.GET("/products", productHandler.GetProducts)
	api.GET("/products/:id", productHandler.GetProduct)
	api.POST("/products", productHandler.CreateProduct)
	api.PUT("/products/:id", productHandler.UpdateProduct)
	api.DELETE("/products/:id", productHandler.DeleteProduct)
	api.DELETE("/products", productHandler.DeleteAllProducts)

	return router
}
