package shopping

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
)

func (h *Handler) CreateShare(c *gin.Context) {
	h.run(c, true, false, http.StatusCreated, func(ctx context.Context, user, list, _ int64) (any, error) {
		var req CreateShareRequest
		if err := decodeBody(c, &req, "token", "preserveMarks"); err != nil {
			return nil, bodyError(err)
		}
		return h.service.CreateShare(ctx, user, list, c.GetHeader("Idempotency-Key"), req)
	})
}

func (h *Handler) ImportShare(c *gin.Context) {
	h.run(c, false, false, http.StatusCreated, func(ctx context.Context, user, _, _ int64) (any, error) {
		var req ImportShareRequest
		if err := decodeBody(c, &req, "token"); err != nil {
			return nil, bodyError(err)
		}
		return h.service.ImportShare(ctx, user, c.GetHeader("Idempotency-Key"), req)
	})
}
