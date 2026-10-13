package shopping

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

type transactionStore struct {
	Store
	tx       Transaction
	beginErr error
}

func (s transactionStore) InTx(_ context.Context, fn func(Transaction) error) error {
	if s.beginErr != nil {
		return s.beginErr
	}
	return fn(s.tx)
}

type replayTransaction struct {
	Transaction
	replay     Replay
	errAt      string
	err        error
	saved      bool
	normalized string
}

func (tx *replayTransaction) fail(at string) error {
	if tx.errAt == at {
		return tx.err
	}
	return nil
}
func (tx *replayTransaction) Claim(_ context.Context, _ int64, _ string, _ string, hash string, _ time.Time) (Replay, error) {
	if tx.replay.Hash == "" {
		tx.replay.Hash = hash
	}
	return tx.replay, tx.fail("claim")
}
func (tx *replayTransaction) CreateList(_ context.Context, _ int64, req CreateListRequest) (List, error) {
	tx.normalized = req.Name
	return List{ID: 1, Name: req.Name}, tx.fail("create")
}
func (tx *replayTransaction) CreateItem(_ context.Context, _ int64, list int64, req CreateItemRequest) (Item, error) {
	tx.normalized = req.Name
	return Item{ID: 1, ListID: list, Name: req.Name, Quantity: req.Quantity}, tx.fail("create")
}
func (tx *replayTransaction) SaveReplay(context.Context, int64, string, string, int64, []byte) error {
	tx.saved = true
	return tx.fail("save")
}
func (tx *replayTransaction) GetList(context.Context, int64, int64) (List, error) {
	return List{ID: 1}, tx.fail("get")
}
func (tx *replayTransaction) GetItem(context.Context, int64, int64, int64) (Item, error) {
	return Item{ID: 1}, tx.fail("get")
}
func (tx *replayTransaction) LockList(context.Context, int64, int64) error  { return tx.fail("lock") }
func (tx *replayTransaction) TouchList(context.Context, int64, int64) error { return tx.fail("touch") }

func TestCreateServicePropagatesFailuresAndReplays(t *testing.T) {
	ctx := context.Background()
	sentinel := errors.New("storage failure")
	for _, itemMode := range []bool{false, true} {
		run := func(tx *replayTransaction, begin error) (any, error) {
			s := NewService(transactionStore{tx: tx, beginErr: begin})
			if itemMode {
				return s.CreateItem(ctx, 7, 9, "service-action-01", CreateItemRequest{Name: " Нормализовано "})
			}
			return s.CreateList(ctx, 7, "service-action-01", CreateListRequest{Name: " Нормализовано "})
		}
		if _, err := run(nil, sentinel); !errors.Is(err, sentinel) {
			t.Fatal("Begin failure swallowed")
		}
		stages := []string{"claim", "create", "save"}
		if itemMode {
			stages = append(stages, "lock", "touch")
		}
		for _, at := range stages {
			tx := &replayTransaction{errAt: at, err: sentinel}
			if _, err := run(tx, nil); !errors.Is(err, sentinel) {
				t.Fatalf("%s failure swallowed: %v", at, err)
			}
		}
		tx := &replayTransaction{}
		if _, err := run(tx, nil); err != nil || !tx.saved || tx.normalized != "Нормализовано" {
			t.Fatalf("Create not normalized/saved: %+v %v", tx, err)
		}
		tx = &replayTransaction{replay: Replay{Hash: "different"}}
		if _, err := run(tx, nil); !errors.Is(err, ErrIdempotencyConflict) {
			t.Fatal("Hash conflict lost")
		}
		for _, test := range []struct{ err, want error }{{ErrNotFound, ErrResourceDeleted}, {sentinel, sentinel}} {
			tx = &replayTransaction{replay: Replay{ResourceID: ptr(int64(1))}, errAt: "get", err: test.err}
			if _, err := run(tx, nil); !errors.Is(err, test.want) {
				t.Fatalf("Replay existence error: %v", err)
			}
		}
		tx = &replayTransaction{replay: Replay{ResourceID: ptr(int64(1)), Response: []byte("invalid JSON")}}
		if _, err := run(tx, nil); err == nil {
			t.Fatal("Corrupt replay accepted")
		}
		original := any(List{ID: 42, Name: "Исходное имя"})
		if itemMode {
			original = Item{ID: 42, ListID: 9, Name: "Исходное имя"}
		}
		payload, err := json.Marshal(original)
		if err != nil {
			t.Fatal(err)
		}
		tx = &replayTransaction{replay: Replay{ResourceID: ptr(int64(42)), Response: payload}}
		result, err := run(tx, nil)
		if err != nil || tx.saved {
			t.Fatal("Replay recreated resource", err)
		}
		got, _ := json.Marshal(result)
		if string(got) != string(payload) {
			t.Fatal("Replay changed DTO")
		}
	}
}
func TestServiceValidationHappensBeforeTransaction(t *testing.T) {
	s := NewService(&forbiddenStore{t: t})
	ctx := context.Background()
	operations := []func() error{
		func() error { _, err := s.CreateList(ctx, 7, "bad", CreateListRequest{Name: "A"}); return err },
		func() error { _, err := s.CreateList(ctx, 7, "valid-action-0001", CreateListRequest{}); return err },
		func() error { _, err := s.UpdateList(ctx, 7, 1, UpdateListRequest{}); return err },
		func() error { _, err := s.UpdateList(ctx, 7, 1, UpdateListRequest{Name: field(" ")}); return err },
		func() error { _, err := s.CreateItem(ctx, 7, 1, "bad", CreateItemRequest{Name: "A"}); return err },
		func() error { _, err := s.CreateItem(ctx, 7, 1, "valid-action-0001", CreateItemRequest{}); return err },
		func() error { _, err := s.UpdateItem(ctx, 7, 1, 1, UpdateItemRequest{}); return err },
		func() error { return s.MarkAll(ctx, 7, 1, MarkAllRequest{}) },
		func() error { return s.MarkAll(ctx, 7, 1, MarkAllRequest{IsMarked: PatchField[bool]{Present: true}}) },
	}
	for _, op := range operations {
		var validation *ValidationError
		if err := op(); !errors.As(err, &validation) {
			t.Fatalf("Expected validation, got %v", err)
		}
	}
}
