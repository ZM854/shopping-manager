package media

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
)

type VariantDTO struct {
	Name                 string `json:"name"`
	ContentType          string `json:"contentType"`
	ByteSize             int64  `json:"byteSize"`
	Width                int    `json:"width"`
	Height               int    `json:"height"`
	DurationMilliseconds *int64 `json:"durationMilliseconds"`
}
type MediaDTO struct {
	ID       int64        `json:"id,string"`
	Purpose  Purpose      `json:"purpose"`
	Variants []VariantDTO `json:"variants"`
}
type StatusDTO struct {
	ID        int64      `json:"id,string"`
	Purpose   Purpose    `json:"purpose"`
	State     State      `json:"state"`
	CreatedAt time.Time  `json:"createdAt"`
	ExpiresAt *time.Time `json:"expiresAt"`
	Media     *MediaDTO  `json:"media"`
	ErrorCode *string    `json:"errorCode"`
}

func mediaDTO(f File) *MediaDTO {
	if f.State != Ready && f.State != Attached {
		return nil
	}
	r := &MediaDTO{ID: f.ID, Purpose: f.Purpose, Variants: make([]VariantDTO, 0, len(f.Objects))}
	for _, o := range f.Objects {
		if o.Metadata == nil {
			return nil
		}
		v := o.Metadata
		r.Variants = append(r.Variants, VariantDTO{v.Name, v.ContentType, v.ByteSize, v.Width, v.Height, v.DurationMilliseconds})
	}
	return r
}

type HTTPService interface {
	Reserve(context.Context, int64, string, ReserveRequest) (Reservation, error)
	GetFile(context.Context, int64, int64) (File, error)
	Cancel(context.Context, int64, int64) error
}
type Handler struct {
	metadata HTTPService
	uploader *Uploader
	access   *AccessService
	log      *slog.Logger
}

func NewHandler(s HTTPService, u *Uploader, access *AccessService, log *slog.Logger) *Handler {
	return &Handler{metadata: s, uploader: u, access: access, log: log}
}
func (h *Handler) Register(g *gin.RouterGroup) {
	g.POST("/recipe-media", h.reserve)
	g.POST("/recipe-media/access", h.accessLinks)
	g.GET("/recipe-media/:mediaId", h.status)
	g.DELETE("/recipe-media/:mediaId", h.cancel)
	g.PUT("/recipe-media/:mediaId/content", h.content)
}

var idPattern = regexp.MustCompile(`^[1-9][0-9]{0,18}$`)

func (h *Handler) identity(c *gin.Context, needID bool) (int64, int64, bool) {
	c.Header("Cache-Control", "no-store")
	owner, ok := c.Get("userID")
	v, valid := owner.(int64)
	if !ok || !valid || v <= 0 {
		c.AbortWithStatusJSON(401, gin.H{"code": "unauthorized", "message": "Требуется авторизация"})
		return 0, 0, false
	}
	var id int64
	if needID {
		raw := c.Param("mediaId")
		n, err := strconv.ParseInt(raw, 10, 64)
		if !idPattern.MatchString(raw) || err != nil || n <= 0 {
			h.fail(c, &ValidationError{Field: "mediaId"})
			return 0, 0, false
		}
		id = n
	}
	return v, id, true
}
func (h *Handler) reserve(c *gin.Context) {
	owner, _, ok := h.identity(c, false)
	if !ok {
		return
	}
	req, err := decodeReserve(c)
	if err != nil {
		h.fail(c, err)
		return
	}
	r, err := h.metadata.Reserve(c.Request.Context(), owner, c.GetHeader("Idempotency-Key"), req)
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(201, r)
}
func (h *Handler) status(c *gin.Context) {
	owner, id, ok := h.identity(c, true)
	if !ok {
		return
	}
	f, err := h.metadata.GetFile(c.Request.Context(), owner, id)
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(200, StatusDTO{f.ID, f.Purpose, f.State, f.CreatedAt, f.ExpiresAt, mediaDTO(f), f.ErrorCode})
}
func (h *Handler) cancel(c *gin.Context) {
	owner, id, ok := h.identity(c, true)
	if !ok {
		return
	}
	if err := h.metadata.Cancel(c.Request.Context(), owner, id); err != nil {
		h.fail(c, err)
		return
	}
	c.Status(204)
}
func (h *Handler) content(c *gin.Context) {
	owner, id, ok := h.identity(c, true)
	if !ok {
		return
	}
	typ, _, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if err != nil || typ != "application/octet-stream" {
		h.fail(c, uploadError("unsupported_media_type"))
		return
	}
	if c.Request.ContentLength > VideoInputLimit {
		h.fail(c, uploadError("body_too_large"))
		return
	}
	controller := http.NewResponseController(c.Writer)
	_ = controller.SetReadDeadline(time.Now().Add(h.uploader.receiveTimeout))
	body := http.MaxBytesReader(c.Writer, c.Request.Body, VideoInputLimit)
	f, err := h.uploader.Upload(c.Request.Context(), owner, id, c.Request.ContentLength, &deadlineBody{ReadCloser: body, controller: controller})
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(200, mediaDTO(f))
}

type deadlineBody struct {
	io.ReadCloser
	controller *http.ResponseController
}

func (b *deadlineBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	if err == io.EOF {
		_ = b.controller.SetReadDeadline(time.Time{})
	}
	return n, err
}

func decodeReserve(c *gin.Context) (ReserveRequest, error) {
	data, err := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, 4096))
	if err != nil {
		return ReserveRequest{}, err
	}
	if !utf8.Valid(data) {
		return ReserveRequest{}, uploadError("invalid_request")
	}
	if _, err := decodeMediaObject(data, "purpose", "byteSize", "sha256"); err != nil {
		return ReserveRequest{}, err
	}
	var r ReserveRequest
	if json.Unmarshal(data, &r) != nil {
		return r, uploadError("invalid_request")
	}
	return r, nil
}

func decodeMediaObject(data []byte, allowed ...string) (map[string]json.RawMessage, error) {
	invalid := uploadError("invalid_request")
	d := json.NewDecoder(bytes.NewReader(data))
	t, err := d.Token()
	if err != nil || t != json.Delim('{') {
		return nil, invalid
	}
	fields := make(map[string]json.RawMessage, len(allowed))
	for d.More() {
		t, err := d.Token()
		if err != nil {
			return nil, invalid
		}
		key, ok := t.(string)
		if !ok || fields[key] != nil || !slices.Contains(allowed, key) {
			return nil, invalid
		}
		var raw json.RawMessage
		if d.Decode(&raw) != nil || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return nil, invalid
		}
		fields[key] = raw
	}
	if t, err = d.Token(); err != nil || t != json.Delim('}') {
		return nil, invalid
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return nil, invalid
	}
	return fields, nil
}

func (h *Handler) fail(c *gin.Context, err error) {
	code := "internal_error"
	status := 500
	message := "Ошибка сервера"
	var fields map[string]string
	var v *ValidationError
	var large *http.MaxBytesError
	var pipeline uploadError
	switch {
	case errors.As(err, &v):
		status = 400
		code = "validation_failed"
		message = "Проверьте поля запроса"
		fields = map[string]string{v.Field: "Некорректное значение"}
	case errors.As(err, &large):
		status = 413
		code = "body_too_large"
	case errors.Is(err, ErrNotFound):
		status = 404
		code = "not_found"
	case errors.Is(err, ErrIdempotencyConflict):
		status = 409
		code = "idempotency_conflict"
	case errors.Is(err, ErrResourceDeleted):
		status = 409
		code = "resource_deleted"
	case errors.Is(err, ErrUploadExpired):
		status = 409
		code = "upload_expired"
	case errors.Is(err, ErrContentMismatch):
		status = 409
		code = "upload_content_mismatch"
	case errors.Is(err, ErrUploadInProgress):
		status = 409
		code = "upload_in_progress"
	case errors.Is(err, ErrAlreadyAttached):
		status = 409
		code = "media_already_attached"
	case errors.Is(err, ErrMediaUnavailable):
		status = 409
		code = "media_unavailable"
	case errors.Is(err, ErrMediaNotReady):
		status = 409
		code = "media_not_ready"
	case errors.Is(err, ErrMediaQuota):
		status = 409
		code = "media_quota_exceeded"
	case errors.Is(err, ErrUploadQuota):
		status = 409
		code = "upload_quota_exceeded"
	case errors.As(err, &pipeline):
		code = string(pipeline)
		switch code {
		case "invalid_request":
			status = 400
		case "body_too_large":
			status = 413
		case "unsupported_media_type":
			status = 415
		case "invalid_media", "media_constraints_exceeded", "media_output_too_large":
			status = 422
		case "media_busy":
			status = 429
		case "media_storage_unavailable", "media_processor_unavailable":
			status = 503
		case "media_processing_timeout":
			status = 504
		}
	}
	if status != 500 {
		message = "Не удалось обработать медиафайл"
	}
	if status == 429 || code == "upload_in_progress" {
		c.Header("Retry-After", "2")
	}
	if status == 500 {
		h.log.Error("media request failed", "method", c.Request.Method, "route", c.FullPath())
	}
	response := gin.H{"code": code, "message": message}
	if fields != nil {
		response["fieldErrors"] = fields
	}
	c.JSON(status, response)
}
