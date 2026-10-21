package shopping

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strconv"
	"time"
)

type Replay struct {
	Hash       string
	ResourceID *int64
	Response   []byte
}

type Reader interface {
	GetLists(context.Context, int64) ([]List, error)
	GetList(context.Context, int64, int64) (List, error)
	GetItems(context.Context, int64, int64) ([]Item, error)
	GetItem(context.Context, int64, int64, int64) (Item, error)
}
type Transaction interface {
	Reader
	ShareTransaction
	LockList(context.Context, int64, int64) error
	CreateList(context.Context, int64, CreateListRequest) (List, error)
	UpdateList(context.Context, int64, int64, string) (List, error)
	DeleteList(context.Context, int64, int64) error
	CreateItem(context.Context, int64, int64, CreateItemRequest) (Item, error)
	UpdateItem(context.Context, int64, int64, int64, UpdateItemRequest) (Item, error)
	DeleteItem(context.Context, int64, int64, int64) error
	ClearItems(context.Context, int64, int64) error
	MarkAll(context.Context, int64, int64, bool) error
	TouchList(context.Context, int64, int64) error
	Claim(context.Context, int64, string, string, string, time.Time) (Replay, error)
	SaveReplay(context.Context, int64, string, string, int64, []byte) error
}
type Store interface {
	Reader
	InTx(context.Context, func(Transaction) error) error
}
type Service struct{ store Store }

func NewService(store Store) *Service { return &Service{store: store} }

func (s *Service) GetLists(ctx context.Context, user int64) ([]List, error) {
	return s.store.GetLists(ctx, user)
}
func (s *Service) GetList(ctx context.Context, user, list int64) (List, error) {
	return s.store.GetList(ctx, user, list)
}
func (s *Service) GetItems(ctx context.Context, user, list int64) ([]Item, error) {
	return s.store.GetItems(ctx, user, list)
}
func (s *Service) GetItem(ctx context.Context, user, list, item int64) (Item, error) {
	return s.store.GetItem(ctx, user, list, item)
}

func requestHash(value any) string {
	data, _ := json.Marshal(value)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func (s *Service) CreateList(ctx context.Context, user int64, key string, req CreateListRequest) (List, error) {
	var result List
	if !keyPattern.MatchString(key) {
		return result, invalid("Idempotency-Key")
	}
	name, err := normalizeText(req.Name, "name", MaxListNameLength, true)
	if err != nil {
		return result, err
	}
	req.Name = name
	err = s.store.InTx(ctx, func(tx Transaction) error {
		replay, err := tx.Claim(ctx, user, "lists", key, requestHash(req), time.Now().UTC().Add(24*time.Hour))
		if err != nil {
			return err
		}
		if replay.Hash != requestHash(req) {
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
		result, err = tx.CreateList(ctx, user, req)
		if err != nil {
			return err
		}
		response, err := json.Marshal(result)
		if err != nil {
			return err
		}
		return tx.SaveReplay(ctx, user, "lists", key, result.ID, response)
	})
	return result, err
}

func (s *Service) UpdateList(ctx context.Context, user, list int64, req UpdateListRequest) (List, error) {
	var result List
	if !req.Name.Present || req.Name.Value == nil {
		return result, invalid("name")
	}
	name, err := normalizeText(*req.Name.Value, "name", MaxListNameLength, true)
	if err != nil {
		return result, err
	}
	err = s.store.InTx(ctx, func(tx Transaction) error {
		if err := tx.LockList(ctx, user, list); err != nil {
			return err
		}
		var err error
		result, err = tx.UpdateList(ctx, user, list, name)
		return err
	})
	return result, err
}

func (s *Service) DeleteList(ctx context.Context, user, list int64) error {
	return s.store.InTx(ctx, func(tx Transaction) error {
		if err := tx.LockList(ctx, user, list); err != nil {
			return err
		}
		return tx.DeleteList(ctx, user, list)
	})
}

func (s *Service) CreateItem(ctx context.Context, user, list int64, key string, req CreateItemRequest) (Item, error) {
	var result Item
	if !keyPattern.MatchString(key) {
		return result, invalid("Idempotency-Key")
	}
	req, err := normalizeCreateItem(req)
	if err != nil {
		return result, err
	}
	scope, hash := "items:"+strconv.FormatInt(list, 10), requestHash(req)
	err = s.store.InTx(ctx, func(tx Transaction) error {
		if err := tx.LockList(ctx, user, list); err != nil {
			return err
		}
		replay, err := tx.Claim(ctx, user, scope, key, hash, time.Now().UTC().Add(24*time.Hour))
		if err != nil {
			return err
		}
		if replay.Hash != hash {
			return ErrIdempotencyConflict
		}
		if replay.ResourceID != nil {
			if _, err := tx.GetItem(ctx, user, list, *replay.ResourceID); errors.Is(err, ErrNotFound) {
				return ErrResourceDeleted
			} else if err != nil {
				return err
			}
			return json.Unmarshal(replay.Response, &result)
		}
		result, err = tx.CreateItem(ctx, user, list, req)
		if err != nil {
			return err
		}
		if err := tx.TouchList(ctx, user, list); err != nil {
			return err
		}
		response, err := json.Marshal(result)
		if err != nil {
			return err
		}
		return tx.SaveReplay(ctx, user, scope, key, result.ID, response)
	})
	return result, err
}

func (s *Service) UpdateItem(ctx context.Context, user, list, item int64, req UpdateItemRequest) (Item, error) {
	var result Item
	req, err := normalizeUpdateItem(req)
	if err != nil {
		return result, err
	}
	err = s.mutateItems(ctx, user, list, func(tx Transaction) error {
		var err error
		result, err = tx.UpdateItem(ctx, user, list, item, req)
		return err
	})
	return result, err
}

func (s *Service) mutateItems(ctx context.Context, user, list int64, mutate func(Transaction) error) error {
	return s.store.InTx(ctx, func(tx Transaction) error {
		if err := tx.LockList(ctx, user, list); err != nil {
			return err
		}
		if err := mutate(tx); err != nil {
			return err
		}
		return tx.TouchList(ctx, user, list)
	})
}
func (s *Service) DeleteItem(ctx context.Context, user, list, item int64) error {
	return s.mutateItems(ctx, user, list, func(tx Transaction) error { return tx.DeleteItem(ctx, user, list, item) })
}
func (s *Service) ClearItems(ctx context.Context, user, list int64) error {
	return s.mutateItems(ctx, user, list, func(tx Transaction) error { return tx.ClearItems(ctx, user, list) })
}
func (s *Service) MarkAll(ctx context.Context, user, list int64, req MarkAllRequest) error {
	if !req.IsMarked.Present || req.IsMarked.Value == nil {
		return invalid("isMarked")
	}
	return s.mutateItems(ctx, user, list, func(tx Transaction) error { return tx.MarkAll(ctx, user, list, *req.IsMarked.Value) })
}
