package recipe

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
)

type RecipeService interface {
	Reader
	CreateRecipe(context.Context, int64, string, SaveRequest) (Recipe, error)
	UpdateRecipe(context.Context, int64, int64, SaveRequest) (Recipe, error)
	DeleteRecipe(context.Context, int64, int64) error
}

type Handler struct {
	service RecipeService
	log     *slog.Logger
}

func NewHandler(service RecipeService, log *slog.Logger) *Handler {
	return &Handler{service: service, log: log}
}

func (h *Handler) fail(c *gin.Context, err error) {
	status := http.StatusInternalServerError
	result := ErrorResponse{Code: "internal_error", Message: "Ошибка сервера"}
	var validation *ValidationError
	var tooLarge *http.MaxBytesError
	switch {
	case errors.Is(err, errInvalidJSON):
		status = http.StatusBadRequest
		result = ErrorResponse{Code: "invalid_request", Message: "Некорректный JSON"}
	case errors.As(err, &tooLarge):
		status = http.StatusRequestEntityTooLarge
		result = ErrorResponse{Code: "body_too_large", Message: "Слишком большой запрос"}
	case errors.As(err, &validation):
		status = http.StatusBadRequest
		result = ErrorResponse{Code: "validation_failed", Message: "Проверьте поля запроса", FieldErrors: validation.Fields}
	case errors.Is(err, ErrNotFound):
		status = http.StatusNotFound
		result = ErrorResponse{Code: "not_found", Message: "Рецепт не найден"}
	case errors.Is(err, ErrIdempotencyConflict):
		status = http.StatusConflict
		result = ErrorResponse{Code: "idempotency_conflict", Message: "Ключ уже использован для другого запроса"}
	case errors.Is(err, ErrResourceDeleted):
		status = http.StatusConflict
		result = ErrorResponse{Code: "resource_deleted", Message: "Созданный рецепт удалён"}
	}
	if status == http.StatusInternalServerError {
		h.log.Error("recipe request failed", "method", c.Request.Method, "route", c.FullPath())
	}
	c.JSON(status, result)
}

type action func(context.Context, int64, int64) (any, error)

func (h *Handler) run(c *gin.Context, idRequired bool, status int, fn action) {
	c.Header("Cache-Control", "no-store")
	value, _ := c.Get("userID")
	owner, ok := value.(int64)
	if !ok || owner <= 0 {
		c.JSON(http.StatusUnauthorized, ErrorResponse{Code: "unauthorized", Message: "Требуется авторизация"})
		return
	}
	var id int64
	if idRequired {
		var err error
		id, err = ParseID(c.Param("recipeId"))
		if err != nil {
			h.fail(c, err)
			return
		}
	}
	result, err := fn(c.Request.Context(), owner, id)
	if err != nil {
		h.fail(c, err)
		return
	}
	if status == http.StatusNoContent {
		c.Status(status)
		return
	}
	c.JSON(status, result)
}

func (h *Handler) GetCategories(c *gin.Context) {
	h.run(c, false, http.StatusOK, func(context.Context, int64, int64) (any, error) {
		return Categories(), nil
	})
}

func (h *Handler) GetRecipes(c *gin.Context) {
	h.run(c, false, http.StatusOK, func(ctx context.Context, owner, _ int64) (any, error) {
		return h.service.GetRecipes(ctx, owner)
	})
}

func (h *Handler) GetRecipe(c *gin.Context) {
	h.run(c, true, http.StatusOK, func(ctx context.Context, owner, id int64) (any, error) {
		return h.service.GetRecipe(ctx, owner, id)
	})
}

func (h *Handler) CreateRecipe(c *gin.Context) {
	h.run(c, false, http.StatusCreated, func(ctx context.Context, owner, _ int64) (any, error) {
		req, err := decodeSaveRequest(c)
		if err != nil {
			return nil, err
		}
		return h.service.CreateRecipe(ctx, owner, c.GetHeader("Idempotency-Key"), req)
	})
}

func (h *Handler) UpdateRecipe(c *gin.Context) {
	h.run(c, true, http.StatusOK, func(ctx context.Context, owner, id int64) (any, error) {
		req, err := decodeSaveRequest(c)
		if err != nil {
			return nil, err
		}
		return h.service.UpdateRecipe(ctx, owner, id, req)
	})
}

func (h *Handler) DeleteRecipe(c *gin.Context) {
	h.run(c, true, http.StatusNoContent, func(ctx context.Context, owner, id int64) (any, error) {
		return nil, h.service.DeleteRecipe(ctx, owner, id)
	})
}

func (h *Handler) Register(group *gin.RouterGroup) {
	group.GET("/recipe-categories", h.GetCategories)
	group.GET("/recipes", h.GetRecipes)
	group.POST("/recipes", h.CreateRecipe)
	group.GET("/recipes/:recipeId", h.GetRecipe)
	group.PUT("/recipes/:recipeId", h.UpdateRecipe)
	group.DELETE("/recipes/:recipeId", h.DeleteRecipe)
}
