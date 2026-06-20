package product

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	repository *Repository
}

func (h *Handler) GetProducts(c *gin.Context) {
	products, err := h.repository.GetProducts(c.Request.Context())

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to get products",
		})
	}
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

	product, err := h.repository.GetProduct(c.Request.Context(), id)

	if errors.Is(err, ErrProductNotFound) {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "product not found",
		})
		return
	}

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to get product",
		})
	}

	c.JSON(http.StatusOK, product)

}

func (h *Handler) PostProduct(c *gin.Context) {
	var newProduct CreateProductRequest

	err := c.ShouldBindJSON(&newProduct)

	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid body",
		})
		return
	}

	product, err := h.repository.PostProduct(c.Request.Context(), newProduct)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to post product",
		})
	}

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

	err = c.ShouldBindJSON(&updatedProductReq)

	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid body",
		})
		return
	}

	updatedProduct, err := h.repository.UpdateProduct(c.Request.Context(), id, updatedProductReq)

	if errors.Is(err, ErrProductNotFound) {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "product not found",
		})
		return
	}

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to update product",
		})
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

	err = h.repository.DeleteProduct(c.Request.Context(), id)

	if errors.Is(err, ErrProductNotFound) {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "product not found",
		})
		return
	}

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to delete product",
		})
	}

	c.Status(http.StatusNoContent)
}

func NewHandler(repository *Repository) *Handler {
	return &Handler{
		repository: repository,
	}
}
