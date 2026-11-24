package media

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
)

const (
	AccessTTL          = 5 * time.Minute
	maxAccessItems     = 50
	maxAccessBodyBytes = 16 << 10
)

type AccessItem struct {
	MediaID int64  `json:"mediaId,string"`
	Variant string `json:"variant"`
}

type AccessLink struct {
	AccessItem
	URL       string    `json:"url"`
	ExpiresAt time.Time `json:"expiresAt"`
}

type AccessService struct {
	repo   *Repository
	signer *Storage
}

func NewAccessService(repo *Repository, signer *Storage) *AccessService {
	return &AccessService{repo: repo, signer: signer}
}

func (s *AccessService) Links(ctx context.Context, owner int64, items []AccessItem) ([]AccessLink, error) {
	if err := validateAccessItems(items); err != nil {
		return nil, err
	}
	objects, err := s.repo.findAccessObjects(ctx, owner, items)
	if err != nil {
		return nil, safeMetadataError(err)
	}
	for _, object := range objects {
		switch {
		case object.key == "":
			return nil, ErrNotFound
		case object.state == DeletePending:
			return nil, ErrMediaUnavailable
		case object.expired:
			return nil, ErrUploadExpired
		case object.state != Ready && object.state != Attached, !object.hasMetadata:
			return nil, ErrMediaNotReady
		}
	}
	result := make([]AccessLink, len(items))
	for i, item := range items {
		url, expires, err := s.signer.SignGet(ctx, objects[i].key)
		if err != nil {
			return nil, uploadError("media_storage_unavailable")
		}
		result[i] = AccessLink{AccessItem: item, URL: url, ExpiresAt: expires}
	}
	return result, nil
}

type accessObject struct {
	key         string
	state       State
	expired     bool
	hasMetadata bool
}

func (r *Repository) findAccessObjects(ctx context.Context, owner int64, items []AccessItem) ([]accessObject, error) {
	ids := make([]int64, len(items))
	names := make([]string, len(items))
	for i, item := range items {
		ids[i], names[i] = item.MediaID, item.Variant
	}
	rows, err := r.pool.Query(ctx, `
		SELECT COALESCE(v.object_key, ''), COALESCE(m.state, ''),
		       COALESCE(m.expires_at <= statement_timestamp(), false), v.content_type IS NOT NULL
		FROM unnest($2::bigint[], $3::text[]) WITH ORDINALITY AS requested(id, name, position)
		LEFT JOIN recipe_media m ON m.id = requested.id AND m.owner_id = $1
		LEFT JOIN recipe_media_variants v ON v.media_id = m.id AND v.name = requested.name
		ORDER BY requested.position`, owner, ids, names)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]accessObject, 0, len(items))
	for rows.Next() {
		var row accessObject
		if err := rows.Scan(&row.key, &row.state, &row.expired, &row.hasMetadata); err != nil {
			return nil, err
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

func validateAccessItems(items []AccessItem) error {
	if len(items) == 0 || len(items) > maxAccessItems {
		return &ValidationError{Field: "items"}
	}
	seen := make(map[AccessItem]bool, len(items))
	for i, item := range items {
		path := "items." + strconv.Itoa(i)
		if item.MediaID <= 0 {
			return &ValidationError{Field: path + ".mediaId"}
		}
		switch item.Variant {
		case "cover", "image", "video", "poster":
		default:
			return &ValidationError{Field: path + ".variant"}
		}
		if seen[item] {
			return &ValidationError{Field: path}
		}
		seen[item] = true
	}
	return nil
}

func (h *Handler) accessLinks(c *gin.Context) {
	owner, _, ok := h.identity(c, false)
	if !ok {
		return
	}
	items, err := decodeAccess(c)
	if err != nil {
		h.fail(c, err)
		return
	}
	links, err := h.access.Links(c.Request.Context(), owner, items)
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": links})
}

func decodeAccess(c *gin.Context) ([]AccessItem, error) {
	data, err := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, maxAccessBodyBytes))
	if err != nil {
		return nil, err
	}
	if !utf8.Valid(data) {
		return nil, uploadError("invalid_request")
	}
	fields, err := decodeMediaObject(data, "items")
	if err != nil {
		return nil, err
	}
	if fields["items"] == nil {
		return nil, &ValidationError{Field: "items"}
	}
	var rawItems []json.RawMessage
	if json.Unmarshal(fields["items"], &rawItems) != nil {
		return nil, uploadError("invalid_request")
	}
	if len(rawItems) == 0 || len(rawItems) > maxAccessItems {
		return nil, &ValidationError{Field: "items"}
	}
	items := make([]AccessItem, len(rawItems))
	for i, rawItem := range rawItems {
		field, err := decodeMediaObject(rawItem, "mediaId", "variant")
		if err != nil {
			return nil, err
		}
		var id string
		if (field["mediaId"] != nil && json.Unmarshal(field["mediaId"], &id) != nil) ||
			(field["variant"] != nil && json.Unmarshal(field["variant"], &items[i].Variant) != nil) {
			return nil, uploadError("invalid_request")
		}
		if idPattern.MatchString(id) {
			if n, err := strconv.ParseInt(id, 10, 64); err == nil {
				items[i].MediaID = n
			}
		}
	}
	return items, nil
}
