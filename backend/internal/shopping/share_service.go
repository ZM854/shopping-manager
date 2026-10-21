package shopping

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strconv"
	"time"
)

var ErrShareTokenConflict = errors.New("share token already used")


type ShareTransaction interface {
	GetShare(context.Context, int64, int64) (Share, error)
	CreateShare(context.Context, int64, int64, []byte, bool) (Share, error)
	CopyListToShare(context.Context, int64, int64, int64) error
	FindShare(context.Context, []byte) (int64, string, error)
	CopyShareToList(context.Context, int64, int64, int64) error
}

func hashShareToken(token string) ([]byte, error) {
	if len(token) != 43 {
		return nil, invalid("token")
	}
	decoded, err := base64.RawURLEncoding.Strict().DecodeString(token)
	if err != nil || len(decoded) != 32 || base64.RawURLEncoding.EncodeToString(decoded) != token {
		return nil, invalid("token")
	}
	hash := sha256.Sum256(decoded)
	return hash[:], nil
}

type shareStorageError struct {
	operation string
	cause     error
}

func (e *shareStorageError) Error() string { return "shopping share " + e.operation + " failed" }
func (e *shareStorageError) Unwrap() error { return e.cause }

func safeShareError(operation string, err error) error {
	switch err {
	case nil, ErrNotFound, ErrResourceDeleted, ErrIdempotencyConflict, ErrShareTokenConflict:
		return err
	}
	return &shareStorageError{operation: operation, cause: err}
}

func (s *Service) CreateShare(
	ctx context.Context, user, list int64, key string, req CreateShareRequest,
) (Share, error) {
	if user <= 0 || list <= 0 {
		return Share{}, invalid("id")
	}
	if !keyPattern.MatchString(key) {
		return Share{}, invalid("Idempotency-Key")
	}
	tokenHash, err := hashShareToken(req.Token)
	if err != nil {
		return Share{}, err
	}
	if req.PreserveMarks == nil {
		return Share{}, invalid("preserveMarks")
	}
	preserve := *req.PreserveMarks
	req.PreserveMarks = &preserve
	scope, hash := "shares:"+strconv.FormatInt(list, 10), requestHash(req)
	var result Share
	err = s.store.InTx(ctx, func(tx Transaction) error {
		replay, err := tx.Claim(ctx, user, scope, key, hash, time.Now().UTC().Add(24*time.Hour))
		if err != nil {
			return err
		}
		if replay.Hash != hash {
			return ErrIdempotencyConflict
		}
		if replay.ResourceID != nil {
			if _, err := tx.GetShare(ctx, user, *replay.ResourceID); errors.Is(err, ErrNotFound) {
				return ErrResourceDeleted
			} else if err != nil {
				return err
			}
			return json.Unmarshal(replay.Response, &result)
		}
		if err := tx.LockList(ctx, user, list); err != nil {
			return err
		}
		result, err = tx.CreateShare(ctx, user, list, tokenHash, preserve)
		if err != nil {
			return err
		}
		if err := tx.CopyListToShare(ctx, user, list, result.ID); err != nil {
			return err
		}
		response, err := json.Marshal(result)
		if err != nil {
			return err
		}
		return tx.SaveReplay(ctx, user, scope, key, result.ID, response)
	})
	if err != nil {
		return Share{}, safeShareError("create", err)
	}
	return result, nil
}

func (s *Service) ImportShare(
	ctx context.Context, user int64, key string, req ImportShareRequest,
) (List, error) {
	if user <= 0 {
		return List{}, invalid("id")
	}
	if !keyPattern.MatchString(key) {
		return List{}, invalid("Idempotency-Key")
	}
	tokenHash, err := hashShareToken(req.Token)
	if err != nil {
		return List{}, err
	}
	const scope = "share-imports"
	hash := requestHash(req)
	var result List
	err = s.store.InTx(ctx, func(tx Transaction) error {
		replay, err := tx.Claim(ctx, user, scope, key, hash, time.Now().UTC().Add(24*time.Hour))
		if err != nil {
			return err
		}
		if replay.Hash != hash {
			return ErrIdempotencyConflict
		}
		if replay.ResourceID != nil {
			if _, err := tx.GetList(ctx, user, *replay.ResourceID); errors.Is(err, ErrNotFound) {
				return ErrResourceDeleted
			} else if err != nil {
				return err
			}
			return json.Unmarshal(replay.Response, &result)
		}
		share, name, err := tx.FindShare(ctx, tokenHash)
		if err != nil {
			return err
		}
		result, err = tx.CreateList(ctx, user, CreateListRequest{Name: name})
		if err != nil {
			return err
		}
		if err := tx.CopyShareToList(ctx, user, result.ID, share); err != nil {
			return err
		}
		response, err := json.Marshal(result)
		if err != nil {
			return err
		}
		return tx.SaveReplay(ctx, user, scope, key, result.ID, response)
	})
	if err != nil {
		return List{}, safeShareError("import", err)
	}
	return result, nil
}
