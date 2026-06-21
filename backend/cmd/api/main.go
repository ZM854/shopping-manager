package main

import (
	"os"
	"time"

	"github.com/ZM854/shopping-manager/backend/internal/config"
	"github.com/ZM854/shopping-manager/backend/internal/database"
	"github.com/ZM854/shopping-manager/backend/internal/logger"
	"github.com/ZM854/shopping-manager/backend/internal/product"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

func main() {
	cfg := config.Load()

	appEnv := cfg.AppENV

	log := logger.New(appEnv)

	log.Info("starting application", "env", appEnv)

	db, err := database.NewPostgres(cfg, log)
	if err != nil {
		log.Error("failed to startup application", "error", err)
		os.Exit(1)
	}
	defer func() {
		log.Info("closing postgres connection pool")
		db.Close()
	}()
	
	log.Info("initializing dependencies")

	repo := product.NewRepository(db, log)
	productHandler := product.NewHandler(repo, log)


	router := gin.Default()

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
		MaxAge: 12 * time.Hour,
	}))

	router.GET("/products", productHandler.GetProducts)
	router.GET("/products/:id", productHandler.GetProduct)
	router.POST("/products", productHandler.PostProduct)
	router.PUT("/products/:id", productHandler.UpdateProduct)
	router.DELETE("/products/:id", productHandler.DeleteProduct)

	addr := cfg.ServerPort

	log.Info("http server started", "addres", addr)

	if err := router.Run(addr); err != nil {
		log.Error("http server stopped", "error", err)
		os.Exit(1)
	}
}
