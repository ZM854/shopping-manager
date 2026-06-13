package product

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	repository *Repository
}

func (h *Handler) GetProducts(c *gin.Context) {
	products := h.repository.GetProducts()
	c.JSON(http.StatusOK, products)
}

func (h *Handler) GetProduct(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)

	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid id",
		})
		return
	}

	product, found := h.repository.GetProduct(id)

	if !found {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "product not found",
		})
		return
	}

	c.JSON(http.StatusOK, product)

}

func (h *Handler) PostProduct(c *gin.Context) {
	var newProduct CreateProductRequest

	err := c.BindJSON(&newProduct)

	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid body",
		})
		return
	}

	product := h.repository.PostProduct(newProduct)
	c.JSON(http.StatusCreated, product)
}

func (h *Handler) UpdateProduct(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)

	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid id",
		})
		return
	}

	var updatedProductReq UpdateProductRequest

	err = c.BindJSON(&updatedProductReq)

	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid body",
		})
		return
	}

	updatedProduct, found := h.repository.UpdateProduct(id, updatedProductReq)

	if !found {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "error while updating product",
		})
		return
	}

	c.JSON(http.StatusOK, updatedProduct)
}

func (h *Handler) DeleteProduct(c *gin.Context) {
idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)

	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid id",
		})
		return
	}

	deleted := h.repository.DeleteProduct(id)

	if !deleted {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "error while deleting product",
		})
		return
	}

	c.Status(http.StatusNoContent)
}

func NewHandler(repository *Repository) *Handler {
	return &Handler{
		repository: repository,
	}
}
