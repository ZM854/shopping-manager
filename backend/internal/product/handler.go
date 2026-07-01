package product

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

type PrductHandler struct {
	repository *ProductRepository
	log *slog.Logger
}

func (h *PrductHandler) GetProducts(c *gin.Context) {
	products, err := h.repository.GetProducts(c.Request.Context())

	if err != nil {
		h.log.Error("failed to get products", "error", err)

		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to get products",
		})
		return
	}
	c.JSON(http.StatusOK, products)
}

func (h *PrductHandler) GetProduct(c *gin.Context) {
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
		h.log.Error("failed to get product", "product_id", id, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to get product",
		})
		return
	}

	c.JSON(http.StatusOK, product)
}

func (h *PrductHandler) PostProduct(c *gin.Context) {
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
		h.log.Error("failed to post product", "product_name", newProduct.Name, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to post product",
		})
		return
	}

	c.JSON(http.StatusCreated, product)
}

func (h *PrductHandler) UpdateProduct(c *gin.Context) {
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
		h.log.Error("failed to update product", "product_id", id, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to update product",
		})
		return
	}

	c.JSON(http.StatusOK, updatedProduct)
}

func (h *PrductHandler) DeleteProduct(c *gin.Context) {
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
		h.log.Error("failed to delete product", "product_id", id, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to delete product",
		})
		return
	}

	c.Status(http.StatusNoContent)
}

func NewHandler(repository *ProductRepository, log *slog.Logger) *PrductHandler {
	return &PrductHandler{
		repository: repository,
		log: log.With("component", "handler", "entity", "product"),
	}
}
