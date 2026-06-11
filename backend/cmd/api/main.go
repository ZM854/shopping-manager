package main

import (
	"github.com/ZM854/shopping-manager/backend/internal/product"
	"github.com/gin-gonic/gin"
)

func main() {
	repo := product.NewRepository()
	productHandler := product.NewHandler(repo)

	router := gin.Default()

	router.GET("/products", productHandler.GetProducts)
	router.GET("/products/:id", productHandler.GetProduct)
	router.POST("/products", productHandler.PostProduct)
	router.PUT("/products", productHandler.UpdateProduct)
	router.DELETE("/products", productHandler.DeleteProduct)

	router.Run()
}
