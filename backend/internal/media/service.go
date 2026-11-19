package media

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"regexp"
	"strconv"
	"time"
)

var keyPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{16,128}$`)
var hashPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

type Service struct{ store Store }

func NewService(store Store) *Service { return &Service{store: store} }

func outputBudget(p Purpose) int64 {
	switch p {
	case Cover:
		return previewLimit
	case StepImage:
		return 1500 << 10
	case StepVideo:
		return videoOutputLimit + previewLimit
	}
	return 0
}

func variantNames(p Purpose) []string {
	switch p {
	case Cover:
		return []string{"cover"}
	case StepImage:
		return []string{"image"}
	case StepVideo:
		return []string{"video", "poster"}
	}
	return nil
}

func validateReserve(key string, req ReserveRequest) error {
	if !keyPattern.MatchString(key) {
		return &ValidationError{Field: "idempotencyKey"}
	}
	if outputBudget(req.Purpose) == 0 {
		return &ValidationError{Field: "purpose"}
	}
	max := inputLimit(req.Purpose)
	if req.ByteSize < 1 || req.ByteSize > max {
		return &ValidationError{Field: "byteSize"}
	}
	if !hashPattern.MatchString(req.SHA256) {
		return &ValidationError{Field: "sha256"}
	}
	return nil
}

func (s *Service) Reserve(ctx context.Context, owner int64, key string, req ReserveRequest) (Reservation, error) {
	if err := validateReserve(key, req); err != nil {
		return Reservation{}, err
	}
	body, err := json.Marshal(req)
	if err != nil {
		return Reservation{}, err
	}
	sum := sha256.Sum256(body)
	hash := hex.EncodeToString(sum[:])
	var result Reservation
	err = s.store.InTx(ctx, func(tx Transaction) error {
		if err := tx.LockOwner(ctx, owner); err != nil {
			return err
		}
		replay, err := tx.Claim(ctx, owner, key, hash)
		if err != nil {
			return err
		}
		if replay.Hash != hash {
			return ErrIdempotencyConflict
		}
		if replay.ResourceID != nil {
			f, err := tx.GetFile(ctx, owner, *replay.ResourceID)
			if errors.Is(err, ErrNotFound) {
				return ErrResourceDeleted
			}
			if err != nil {
				return err
			}
			if f.State == DeletePending {
				return ErrResourceDeleted
			}
			now, err := tx.Now(ctx)
			if err != nil {
				return err
			}
			if expired(f, now) {
				return ErrUploadExpired
			}
			if err := json.Unmarshal(replay.Response, &result); err != nil {
				return err
			}
			if result.ID != f.ID || result.Purpose != req.Purpose || result.ByteSize != req.ByteSize || result.SHA256 != req.SHA256 {
				return errors.New("invalid media reservation replay")
			}
			return nil
		}
		u, err := tx.Usage(ctx, owner)
		if err != nil {
			return err
		}
		if u.AccountedBytes > OwnerBytesLimit-outputBudget(req.Purpose) {
			return ErrMediaQuota
		}
		if u.SourceBytes > SourceBytesLimit-req.ByteSize || u.UploadSlots >= UploadSlotsLimit {
			return ErrUploadQuota
		}
		f, err := tx.CreateFile(ctx, owner, req)
		if err != nil {
			return err
		}
		result = Reservation{ID: f.ID, Purpose: f.Purpose, ByteSize: f.ByteSize, SHA256: f.SHA256, CreatedAt: f.CreatedAt, ExpiresAt: *f.ExpiresAt}
		response, err := json.Marshal(result)
		if err != nil {
			return err
		}
		return tx.SaveReplay(ctx, owner, key, result, response)
	})
	if err != nil {
		return Reservation{}, safeMetadataError(err)
	}
	return result, nil
}

func expired(f File, now time.Time) bool { return f.ExpiresAt != nil && !now.Before(*f.ExpiresAt) }


func (s *Service) withFile(ctx context.Context, owner, id int64, write bool, fn func(Transaction, *File, time.Time) error) (File, error) {
	var result File
	err := s.store.InTx(ctx, func(tx Transaction) error {
		if err := tx.LockOwner(ctx, owner); err != nil {
			return err
		}
		f, err := tx.GetFile(ctx, owner, id)
		if err != nil {
			return err
		}
		now, err := tx.Now(ctx)
		if err != nil {
			return err
		}
		if err := fn(tx, &f, now); err != nil {
			return err
		}
		if write {
			if err := tx.SaveFile(ctx, f); err != nil {
				return err
			}
			f, err = tx.GetFile(ctx, owner, id)
			if err != nil {
				return err
			}
		}
		result = f
		return nil
	})
	if err != nil {
		return File{}, safeMetadataError(err)
	}
	return result, nil
}

func (s *Service) GetFile(ctx context.Context, owner, id int64) (File, error) {
	return s.withFile(ctx, owner, id, false, func(_ Transaction, f *File, now time.Time) error {
		if f.State != DeletePending && expired(*f, now) {
			return ErrUploadExpired
		}
		return nil
	})
}

func (s *Service) BeginProcessing(ctx context.Context, owner, id, size int64, hash string) (File, error) {
	return s.withFile(ctx, owner, id, true, func(tx Transaction, f *File, now time.Time) error {
		if f.State == DeletePending {
			return ErrMediaUnavailable
		}
		if expired(*f, now) {
			return ErrUploadExpired
		}
		if f.State == Processing {
			return ErrUploadInProgress
		}
		if f.ByteSize != size || f.SHA256 != hash {
			return ErrContentMismatch
		}
		if f.State == Ready || f.State == Attached {
			return nil
		}
		if f.State != AwaitingUpload && f.State != Failed {
			return ErrMediaNotReady
		}
		if f.SourceReservedBytes == 0 {
			u, err := tx.Usage(ctx, owner)
			if err != nil {
				return err
			}
			if u.SourceBytes > SourceBytesLimit-f.ByteSize {
				return ErrUploadQuota
			}
			f.SourceReservedBytes = f.ByteSize
		}
		f.State = Processing
		f.ErrorCode = nil
		return nil
	})
}

func (s *Service) Complete(ctx context.Context, owner, id int64, variants []Variant) (File, error) {
	var outcome error
	f, err := s.withFile(ctx, owner, id, true, func(_ Transaction, f *File, now time.Time) error {
		if f.State != Processing {
			return ErrMediaNotReady
		}
		if f.SourceReservedBytes != 0 {
			return ErrUploadInProgress
		}
		if expired(*f, now) {
			f.State = DeletePending
			f.ErrorCode = nil
			outcome = ErrUploadExpired
			return nil
		}
		objects, total, err := completedObjects(*f, variants)
		if err != nil {
			return err
		}
		f.Objects = objects
		f.AccountedBytes = total
		f.State = Ready
		f.ErrorCode = nil
		return nil
	})
	if err != nil {
		return File{}, err
	}
	if outcome != nil {
		return File{}, outcome
	}
	return f, nil
}

func (s *Service) Fail(ctx context.Context, owner, id int64, code string) (File, error) {
	if !validFailureCode(code) {
		return File{}, &ValidationError{Field: "errorCode"}
	}
	return s.withFile(ctx, owner, id, true, func(_ Transaction, f *File, now time.Time) error {
		if f.State != Processing {
			return ErrMediaNotReady
		}
		if f.SourceReservedBytes != 0 {
			return ErrUploadInProgress
		}
		f.State = Failed
		f.ErrorCode = &code
		if expired(*f, now) {
			f.State = DeletePending
			f.ErrorCode = nil
		}
		return nil
	})
}

func validFailureCode(code string) bool {
	switch code {
	case "invalid_media", "unsupported_media_type", "media_constraints_exceeded", "media_output_too_large",
		"media_storage_unavailable", "media_processor_unavailable", "media_processing_timeout", "media_processing_interrupted":
		return true
	}
	return false
}

func (s *Service) Cancel(ctx context.Context, owner, id int64) error {
	_, err := s.withFile(ctx, owner, id, true, func(_ Transaction, f *File, _ time.Time) error {
		if f.State == Processing {
			return ErrUploadInProgress
		}
		if f.State == Attached {
			return ErrAlreadyAttached
		}
		f.State = DeletePending
		f.ErrorCode = nil
		return nil
	})
	return err
}

func (s *Service) ReleaseSource(ctx context.Context, owner, id int64) error {
	_, err := s.withFile(ctx, owner, id, true, func(_ Transaction, f *File, _ time.Time) error { f.SourceReservedBytes = 0; return nil })
	return err
}

func (s *Service) ConfirmDeletion(ctx context.Context, owner, id int64) error {
	err := s.store.InTx(ctx, func(tx Transaction) error {
		if err := tx.LockOwner(ctx, owner); err != nil {
			return err
		}
		f, err := tx.GetFile(ctx, owner, id)
		if err != nil {
			return err
		}
		if f.State != DeletePending {
			return ErrMediaUnavailable
		}
		if f.SourceReservedBytes != 0 {
			return ErrUploadInProgress
		}
		return tx.DeleteFile(ctx, owner, id)
	})
	return safeMetadataError(err)
}

func completedObjects(f File, variants []Variant) ([]Object, int64, error) {
	names := variantNames(f.Purpose)
	if len(variants) != len(names) || len(f.Objects) != len(names) {
		return nil, 0, &ValidationError{Field: "variants"}
	}
	objects := make([]Object, len(names))
	var total int64
	for i, v := range variants {
		if v.Name != names[i] || f.Objects[i].Name != names[i] || !validVariant(v) {
			return nil, 0, &ValidationError{Field: "variants." + strconv.Itoa(i)}
		}
		copy := v
		if v.DurationMilliseconds != nil {
			duration := *v.DurationMilliseconds
			copy.DurationMilliseconds = &duration
		}
		objects[i] = Object{Name: v.Name, Key: f.Objects[i].Key, Metadata: &copy}
		total += v.ByteSize
	}
	return objects, total, nil
}

func validVariant(v Variant) bool {
	if v.ByteSize < 1 || v.Width < 1 || v.Height < 1 {
		return false
	}
	if v.Name == "video" {
		return v.ContentType == "video/mp4" && v.ByteSize <= videoOutputLimit && v.Width%2 == 0 && v.Height%2 == 0 &&
			((v.Width <= 1280 && v.Height <= 720) || (v.Width <= 720 && v.Height <= 1280)) &&
			v.DurationMilliseconds != nil && *v.DurationMilliseconds >= 1 && *v.DurationMilliseconds <= 60000
	}
	if v.ContentType != "image/jpeg" || v.DurationMilliseconds != nil {
		return false
	}
	switch v.Name {
	case "cover":
		return v.ByteSize <= previewLimit && v.Width == 640 && v.Height == 360
	case "image":
		return v.ByteSize <= 1500<<10 && v.Width <= 1600 && v.Height <= 1600
	case "poster":
		return v.ByteSize <= previewLimit && ((v.Width <= 640 && v.Height <= 360) || (v.Width <= 360 && v.Height <= 640))
	}
	return false
}

type metadataError struct{ cause error }

func (e *metadataError) Error() string { return "media metadata operation failed" }
func (e *metadataError) Unwrap() error { return e.cause }
func safeMetadataError(err error) error {
	if err == nil {
		return nil
	}
	var validation *ValidationError
	if errors.As(err, &validation) {
		return err
	}
	for _, domain := range []error{ErrNotFound, ErrIdempotencyConflict, ErrResourceDeleted, ErrUploadExpired, ErrContentMismatch,
		ErrUploadInProgress, ErrMediaNotReady, ErrMediaUnavailable, ErrAlreadyAttached, ErrMediaQuota, ErrUploadQuota} {
		if errors.Is(err, domain) {
			return domain
		}
	}
	return &metadataError{cause: err}
}
