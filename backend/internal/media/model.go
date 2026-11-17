package media

import (
	"errors"
	"time"
)

type Purpose string
type State string

const (
	Cover            Purpose = "cover"
	StepImage        Purpose = "step-image"
	StepVideo        Purpose = "step-video"
	AwaitingUpload   State   = "awaiting_upload"
	Processing       State   = "processing"
	Ready            State   = "ready"
	Attached         State   = "attached"
	Failed           State   = "failed"
	DeletePending    State   = "delete_pending"
	UploadTTL                = 24 * time.Hour
	OwnerBytesLimit  int64   = 1 << 30
	SourceBytesLimit int64   = 200 << 20
	UploadSlotsLimit         = 25
	ImageInputLimit  int64   = 15 << 20
	VideoInputLimit  int64   = 50 << 20
	videoOutputLimit int64   = 20 << 20
	previewLimit     int64   = 200 << 10
)

func inputLimit(p Purpose) int64 {
	switch p {
	case Cover, StepImage:
		return ImageInputLimit
	case StepVideo:
		return VideoInputLimit
	default:
		return 0
	}
}

var (
	ErrNotFound            = errors.New("media not found")
	ErrIdempotencyConflict = errors.New("media idempotency conflict")
	ErrResourceDeleted     = errors.New("media resource deleted")
	ErrUploadExpired       = errors.New("upload expired")
	ErrContentMismatch     = errors.New("upload content mismatch")
	ErrUploadInProgress    = errors.New("upload in progress")
	ErrMediaNotReady       = errors.New("media not ready")
	ErrMediaUnavailable    = errors.New("media unavailable")
	ErrAlreadyAttached     = errors.New("media already attached")
	ErrMediaQuota          = errors.New("media quota exceeded")
	ErrUploadQuota         = errors.New("upload quota exceeded")
)

type ValidationError struct{ Field string }

func (e *ValidationError) Error() string { return "invalid media field: " + e.Field }

type ReserveRequest struct {
	Purpose  Purpose `json:"purpose"`
	ByteSize int64   `json:"byteSize"`
	SHA256   string  `json:"sha256"`
}

// Reservation is the immutable result of POST, never the current file status.
type Reservation struct {
	ID        int64     `json:"id,string"`
	Purpose   Purpose   `json:"purpose"`
	ByteSize  int64     `json:"byteSize"`
	SHA256    string    `json:"sha256"`
	CreatedAt time.Time `json:"createdAt"`
	ExpiresAt time.Time `json:"expiresAt"`
}

type Variant struct {
	Name                 string
	ContentType          string
	ByteSize             int64
	Width                int
	Height               int
	DurationMilliseconds *int64
}

// Object rows also describe planned keys when Metadata is nil. Not an HTTP DTO.
type Object struct {
	Name     string
	Key      string `json:"-"`
	Metadata *Variant
}

// File is internal persistence state, not an HTTP DTO. Handlers expose explicit
// reservation/status/media DTOs, never ownership, accounting or object keys.
type File struct {
	ID                  int64
	OwnerID             *int64
	Purpose             Purpose
	ByteSize            int64
	SHA256              string
	State               State
	CreatedAt           time.Time
	UpdatedAt           time.Time
	ExpiresAt           *time.Time
	ErrorCode           *string
	AccountedBytes      int64
	SourceReservedBytes int64
	UploadSlotReserved  bool
	RecipeID            *int64
	StepID              *int64
	Position            *int
	Objects             []Object `json:"-"`
}

type Usage struct {
	AccountedBytes int64
	SourceBytes    int64
	UploadSlots    int
}

type Replay struct {
	Hash       string
	ResourceID *int64
	Response   []byte
}
