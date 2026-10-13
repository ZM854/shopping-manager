package shopping

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
)

type ShoppingService interface {
	Reader
	CreateList(context.Context, int64, string, CreateListRequest) (List, error)
	UpdateList(context.Context, int64, int64, UpdateListRequest) (List, error)
	DeleteList(context.Context, int64, int64) error
	CreateItem(context.Context, int64, int64, string, CreateItemRequest) (Item, error)
	UpdateItem(context.Context, int64, int64, int64, UpdateItemRequest) (Item, error)
	DeleteItem(context.Context, int64, int64, int64) error
	ClearItems(context.Context, int64, int64) error
	MarkAll(context.Context, int64, int64, MarkAllRequest) error
}
type Handler struct {
	service ShoppingService
	log     *slog.Logger
}

func NewHandler(service ShoppingService, log *slog.Logger) *Handler {
	return &Handler{service: service, log: log}
}

var errInvalidJSON = errors.New("invalid shopping JSON")

func decodeBody(c *gin.Context, dst any, allowed ...string) error {
	data, err := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, MaxJSONBodyBytes))
	if err != nil {
		return err
	}
	if !utf8.Valid(data) {
		return errInvalidJSON
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return errInvalidJSON
	}
	seen := make(map[string]bool)
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return errInvalidJSON
		}
		key, ok := token.(string)
		if !ok || seen[key] {
			return errInvalidJSON
		}
		valid := false
		for _, field := range allowed {
			if key == field {
				valid = true
				break
			}
		}
		if !valid {
			return errInvalidJSON
		}
		seen[key] = true
		var raw json.RawMessage
		if err := decoder.Decode(&raw); err != nil {
			return errInvalidJSON
		}
		if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) && key != "quantity" {
			return errInvalidJSON
		}
	}
	if _, err := decoder.Token(); err != nil {
		return errInvalidJSON
	}
	if _, err := decoder.Token(); err != io.EOF {
		return errInvalidJSON
	}
	if err := json.Unmarshal(data, dst); err != nil {
		return errInvalidJSON
	}
	return nil
}

func (h *Handler) fail(c *gin.Context, err error) {
	status, result := http.StatusInternalServerError, ErrorResponse{Code: "internal_error", Message: "Ошибка сервера"}
	var validation *ValidationError
	var tooLarge *http.MaxBytesError
	switch {
	case errors.Is(err, errInvalidJSON):
		status, result = 400, ErrorResponse{Code: "invalid_request", Message: "Некорректный JSON"}
	case errors.As(err, &tooLarge):
		status, result = 413, ErrorResponse{Code: "body_too_large", Message: "Слишком большой запрос"}
	case errors.As(err, &validation):
		status, result = 400, ErrorResponse{Code: "validation_failed", Message: "Проверьте поля запроса", FieldErrors: validation.Fields}
	case errors.Is(err, ErrNotFound):
		status, result = 404, ErrorResponse{Code: "not_found", Message: "Ресурс не найден"}
	case errors.Is(err, ErrIdempotencyConflict):
		status, result = 409, ErrorResponse{Code: "idempotency_conflict", Message: "Ключ уже использован для другого запроса"}
	case errors.Is(err, ErrResourceDeleted):
		status, result = 409, ErrorResponse{Code: "resource_deleted", Message: "Созданный ресурс удалён"}
	}
	if status == 500 {
		h.log.Error("shopping request failed", "error", err)
	}
	c.JSON(status, result)
}

type action func(context.Context, int64, int64, int64) (any, error)

func (h *Handler) run(c *gin.Context, listRequired, itemRequired bool, status int, fn action) {
	value, _ := c.Get("userID")
	user, ok := value.(int64)
	if !ok || user <= 0 {
		c.JSON(401, ErrorResponse{Code: "unauthorized", Message: "Требуется авторизация"})
		return
	}
	var list, item int64
	var err error
	if listRequired {
		list, err = ParseID(c.Param("listId"))
		if err != nil {
			h.fail(c, err)
			return
		}
	}
	if itemRequired {
		item, err = ParseID(c.Param("itemId"))
		if err != nil {
			h.fail(c, err)
			return
		}
	}
	result, err := fn(c.Request.Context(), user, list, item)
	if err != nil {
		h.fail(c, err)
		return
	}
	if status == http.StatusNoContent {
		c.Status(status)
	} else {
		c.JSON(status, result)
	}
}

func (h *Handler) GetLists(c *gin.Context) {
	h.run(c, false, false, 200, func(ctx context.Context, u, l, i int64) (any, error) { return h.service.GetLists(ctx, u) })
}
func (h *Handler) GetList(c *gin.Context) {
	h.run(c, true, false, 200, func(ctx context.Context, u, l, i int64) (any, error) { return h.service.GetList(ctx, u, l) })
}
func (h *Handler) GetItems(c *gin.Context) {
	h.run(c, true, false, 200, func(ctx context.Context, u, l, i int64) (any, error) { return h.service.GetItems(ctx, u, l) })
}
func (h *Handler) GetItem(c *gin.Context) {
	h.run(c, true, true, 200, func(ctx context.Context, u, l, i int64) (any, error) { return h.service.GetItem(ctx, u, l, i) })
}
func (h *Handler) CreateList(c *gin.Context) {
	h.run(c, false, false, 201, func(ctx context.Context, u, l, i int64) (any, error) {
		var req CreateListRequest
		if err := decodeBody(c, &req, "name"); err != nil {
			return nil, bodyError(err)
		}
		return h.service.CreateList(ctx, u, c.GetHeader("Idempotency-Key"), req)
	})
}
func (h *Handler) UpdateList(c *gin.Context) {
	h.run(c, true, false, 200, func(ctx context.Context, u, l, i int64) (any, error) {
		var req UpdateListRequest
		if err := decodeBody(c, &req, "name"); err != nil {
			return nil, bodyError(err)
		}
		return h.service.UpdateList(ctx, u, l, req)
	})
}
func (h *Handler) CreateItem(c *gin.Context) {
	h.run(c, true, false, 201, func(ctx context.Context, u, l, i int64) (any, error) {
		var req CreateItemRequest
		if err := decodeBody(c, &req, "name", "quantity", "unit", "isMarked"); err != nil {
			return nil, bodyError(err)
		}
		return h.service.CreateItem(ctx, u, l, c.GetHeader("Idempotency-Key"), req)
	})
}
func (h *Handler) UpdateItem(c *gin.Context) {
	h.run(c, true, true, 200, func(ctx context.Context, u, l, i int64) (any, error) {
		var req UpdateItemRequest
		if err := decodeBody(c, &req, "name", "quantity", "unit", "isMarked"); err != nil {
			return nil, bodyError(err)
		}
		return h.service.UpdateItem(ctx, u, l, i, req)
	})
}
func (h *Handler) MarkAll(c *gin.Context) {
	h.run(c, true, false, 204, func(ctx context.Context, u, l, i int64) (any, error) {
		var req MarkAllRequest
		if err := decodeBody(c, &req, "isMarked"); err != nil {
			return nil, bodyError(err)
		}
		return nil, h.service.MarkAll(ctx, u, l, req)
	})
}
func (h *Handler) DeleteList(c *gin.Context) {
	h.run(c, true, false, 204, func(ctx context.Context, u, l, i int64) (any, error) { return nil, h.service.DeleteList(ctx, u, l) })
}
func (h *Handler) DeleteItem(c *gin.Context) {
	h.run(c, true, true, 204, func(ctx context.Context, u, l, i int64) (any, error) { return nil, h.service.DeleteItem(ctx, u, l, i) })
}
func (h *Handler) ClearItems(c *gin.Context) {
	h.run(c, true, false, 204, func(ctx context.Context, u, l, i int64) (any, error) { return nil, h.service.ClearItems(ctx, u, l) })
}

func bodyError(err error) error {
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		return err
	}
	return errInvalidJSON
}

func (h *Handler) Register(group *gin.RouterGroup) {
	group.GET("/shopping-lists", h.GetLists)
	group.POST("/shopping-lists", h.CreateList)
	group.GET("/shopping-lists/:listId", h.GetList)
	group.PATCH("/shopping-lists/:listId", h.UpdateList)
	group.DELETE("/shopping-lists/:listId", h.DeleteList)
	group.GET("/shopping-lists/:listId/items", h.GetItems)
	group.POST("/shopping-lists/:listId/items", h.CreateItem)
	group.DELETE("/shopping-lists/:listId/items", h.ClearItems)
	group.PATCH("/shopping-lists/:listId/items/mark-all", h.MarkAll)
	group.GET("/shopping-lists/:listId/items/:itemId", h.GetItem)
	group.PATCH("/shopping-lists/:listId/items/:itemId", h.UpdateItem)
	group.DELETE("/shopping-lists/:listId/items/:itemId", h.DeleteItem)
}
