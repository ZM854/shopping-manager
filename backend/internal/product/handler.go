package product

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

type ProductHandler struct {
	service *ProductService
	log     *slog.Logger
}

func getUserID(c *gin.Context) (int64, bool) {
	value, exists := c.Get("userID")
	if !exists {
		return 0, false
	}

	userID, ok := value.(int64)
	return userID, ok
}

func (h *ProductHandler) GetProducts(c *gin.Context) {
	userID, ok := getUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "user not authenticated",
		})
		return
	}

	products, err := h.service.GetProducts(c.Request.Context(), userID)
	if err != nil {
		h.log.Error(
			"failed to get products",
			"user_id", userID,
			"error", err,
		)

		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to get products",
		})
		return
	}

	c.JSON(http.StatusOK, products)
}

func (h *ProductHandler) GetProduct(c *gin.Context) {
	userID, ok := getUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "user not authenticated",
		})
		return
	}

	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid id",
		})
		return
	}

	product, err := h.service.GetProduct(
		c.Request.Context(),
		userID,
		id,
	)

	if errors.Is(err, ErrProductNotFound) {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "product not found",
		})
		return
	}

	if err != nil {
		h.log.Error(
			"failed to get product",
			"user_id", userID,
			"product_id", id,
			"error", err,
		)

		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to get product",
		})
		return
	}

	c.JSON(http.StatusOK, product)
}

func (h *ProductHandler) CreateProduct(c *gin.Context) {
	userID, ok := getUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "user not authenticated",
		})
		return
	}

	var req CreateProductRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid body",
		})
		return
	}

	product, err := h.service.CreateProduct(
		c.Request.Context(),
		userID,
		req,
	)

	if err != nil {
		h.log.Error(
			"failed to create product",
			"user_id", userID,
			"product_name", req.Name,
			"error", err,
		)

		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to create product",
		})
		return
	}

	c.JSON(http.StatusCreated, product)
}

func (h *ProductHandler) UpdateProduct(c *gin.Context) {
	userID, ok := getUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "user not authenticated",
		})
		return
	}

	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid id",
		})
		return
	}

	var req UpdateProductRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid body",
		})
		return
	}

	product, err := h.service.UpdateProduct(
		c.Request.Context(),
		userID,
		id,
		req,
	)

	if errors.Is(err, ErrProductNotFound) {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "product not found",
		})
		return
	}

	if err != nil {
		h.log.Error(
			"failed to update product",
			"user_id", userID,
			"product_id", id,
			"error", err,
		)

		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to update product",
		})
		return
	}

	c.JSON(http.StatusOK, product)
}

func (h *ProductHandler) DeleteProduct(c *gin.Context) {
	userID, ok := getUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "user not authenticated",
		})
		return
	}

	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid id",
		})
		return
	}

	err = h.service.DeleteProduct(
		c.Request.Context(),
		userID,
		id,
	)

	if errors.Is(err, ErrProductNotFound) {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "product not found",
		})
		return
	}

	if err != nil {
		h.log.Error(
			"failed to delete product",
			"user_id", userID,
			"product_id", id,
			"error", err,
		)

		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to delete product",
		})
		return
	}

	c.Status(http.StatusNoContent)
}

func NewProductHandler(
	service *ProductService,
	log *slog.Logger,
) *ProductHandler {
	return &ProductHandler{
		service: service,
		log: log.With(
			"component", "handler",
			"entity", "product",
		),
	}
}