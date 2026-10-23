package recipe

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"
)

const replayRetention = 24 * time.Hour

type Service struct {
	store Store
}

func NewService(store Store) *Service {
	return &Service{store: store}
}

func (s *Service) GetRecipes(ctx context.Context, owner int64) ([]Summary, error) {
	result, err := s.store.GetRecipes(ctx, owner)
	if err != nil {
		return nil, safeStorageError(err)
	}
	return result, nil
}

func (s *Service) GetRecipe(ctx context.Context, owner, id int64) (Recipe, error) {
	result, err := s.store.GetRecipe(ctx, owner, id)
	if err != nil {
		return Recipe{}, safeStorageError(err)
	}
	return result, nil
}

func (s *Service) CreateRecipe(ctx context.Context, owner int64, key string, req SaveRequest) (Recipe, error) {
	if err := validateIdempotencyKey(key); err != nil {
		return Recipe{}, err
	}
	req, err := normalizeSaveRequest(req)
	if err != nil {
		return Recipe{}, err
	}
	body, err := json.Marshal(req)
	if err != nil {
		return Recipe{}, err
	}
	sum := sha256.Sum256(body)
	hash := hex.EncodeToString(sum[:])
	var result Recipe
	err = s.store.InTx(ctx, func(tx Transaction) error {
		replay, err := tx.Claim(ctx, owner, key, hash, time.Now().UTC().Add(replayRetention))
		if err != nil {
			return err
		}
		if replay.Hash != hash {
			return ErrIdempotencyConflict
		}
		if replay.ResourceID != nil {
			if _, err := tx.GetRecipe(ctx, owner, *replay.ResourceID); errors.Is(err, ErrNotFound) {
				return ErrResourceDeleted
			} else if err != nil {
				return err
			}
			return json.Unmarshal(replay.Response, &result)
		}
		result, err = tx.CreateRecipe(ctx, owner, req)
		if err != nil {
			return err
		}
		response, err := json.Marshal(result)
		if err != nil {
			return err
		}
		return tx.SaveReplay(ctx, owner, key, result.ID, response)
	})
	if err != nil {
		return Recipe{}, safeStorageError(err)
	}
	return result, nil
}

func (s *Service) UpdateRecipe(ctx context.Context, owner, id int64, req SaveRequest) (Recipe, error) {
	req, err := normalizeSaveRequest(req)
	if err != nil {
		return Recipe{}, err
	}
	var result Recipe
	err = s.store.InTx(ctx, func(tx Transaction) error {
		if err := tx.LockRecipe(ctx, owner, id); err != nil {
			return err
		}
		var err error
		result, err = tx.UpdateRecipe(ctx, owner, id, req)
		return err
	})
	if err != nil {
		return Recipe{}, safeStorageError(err)
	}
	return result, nil
}

func (s *Service) DeleteRecipe(ctx context.Context, owner, id int64) error {
	err := s.store.InTx(ctx, func(tx Transaction) error {
		if err := tx.LockRecipe(ctx, owner, id); err != nil {
			return err
		}
		return tx.DeleteRecipe(ctx, owner, id)
	})
	return safeStorageError(err)
}

type storageError struct{ cause error }

func (e *storageError) Error() string { return "recipe storage operation failed" }
func (e *storageError) Unwrap() error { return e.cause }

func safeStorageError(err error) error {
	if err == nil {
		return nil
	}
	for _, domain := range []error{ErrNotFound, ErrIdempotencyConflict, ErrResourceDeleted} {
		if errors.Is(err, domain) {
			return domain
		}
	}
	return &storageError{cause: err}
}
