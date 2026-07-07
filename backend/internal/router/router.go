package router

import (
	"log/slog"
	"time"

	"github.com/ZM854/shopping-manager/backend/internal/auth"
	"github.com/ZM854/shopping-manager/backend/internal/middleware"
	"github.com/ZM854/shopping-manager/backend/internal/product"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

func New(log *slog.Logger, productHandler *product.PrductHandler, authHandler *auth.AuthHandler) *gin.Engine {
	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(middleware.RequestLogger(log))

	router.Use(cors.New(cors.Config{
		AllowOrigins: []string{
			"http://localhost:5173",
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
		MaxAge: 12 * time.Hour,
	}))

	router.GET("/products", productHandler.GetProducts)
	router.GET("/products/:id", productHandler.GetProduct)
	router.POST("/products", productHandler.PostProduct)
	router.PUT("/products/:id", productHandler.UpdateProduct)
	router.DELETE("/products/:id", productHandler.DeleteProduct)

	router.POST("/registration", authHandler.Registration)
	router.POST("/login", authHandler.Login)
	router.POST("/logout", authHandler.Logout)
	router.GET("/activate/:link", authHandler.Activate)
	router.GET("/refresh", authHandler.Refresh)
	router.GET("/users", authHandler.GetUsers)


	return router
}
