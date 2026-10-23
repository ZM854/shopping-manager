package recipe

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

type serviceStore struct {
	Store
	tx        *serviceTransaction
	beginErr  error
	commitErr error
}

func (s serviceStore) InTx(_ context.Context, fn func(Transaction) error) error {
	if s.beginErr != nil {
		return s.beginErr
	}
	if err := fn(s.tx); err != nil {
		return err
	}
	return s.commitErr
}

type serviceTransaction struct {
	Transaction
	t          *testing.T
	errAt      string
	err        error
	calls      []string
	replay     Replay
	result     Recipe
	normalized SaveRequest
	expires    time.Time
	saved      Recipe
}

func (tx *serviceTransaction) operation(name string, owner int64) error {
	tx.t.Helper()
	if owner != 7 {
		tx.t.Fatalf("Потерян владелец: %d", owner)
	}
	tx.calls = append(tx.calls, name)
	if name == tx.errAt {
		return tx.err
	}
	return nil
}

func (tx *serviceTransaction) Claim(_ context.Context, owner int64, key, hash string, expires time.Time) (Replay, error) {
	if key != "service-recipe-0001" || len(hash) != 64 {
		tx.t.Fatalf("Неверный ключ/хеш: %q, %q", key, hash)
	}
	tx.expires = expires
	replay := tx.replay
	if replay.Hash == "" {
		replay.Hash = hash
	}
	return replay, tx.operation("claim", owner)
}

func (tx *serviceTransaction) CreateRecipe(_ context.Context, owner int64, req SaveRequest) (Recipe, error) {
	tx.normalized = req
	return tx.result, tx.operation("create", owner)
}

func (tx *serviceTransaction) SaveReplay(_ context.Context, owner int64, key string, id int64, body []byte) error {
	if key != "service-recipe-0001" || id != tx.result.ID {
		tx.t.Fatal("Неверная привязка replay")
	}
	if err := json.Unmarshal(body, &tx.saved); err != nil {
		tx.t.Fatal(err)
	}
	return tx.operation("save", owner)
}

func (tx *serviceTransaction) GetRecipe(_ context.Context, owner, id int64) (Recipe, error) {
	if tx.replay.ResourceID == nil || id != *tx.replay.ResourceID {
		tx.t.Fatal("Не проверен доступ к результату replay")
	}
	return tx.result, tx.operation("get", owner)
}

func (tx *serviceTransaction) LockRecipe(_ context.Context, owner, id int64) error {
	if id != 11 {
		tx.t.Fatal("Блокировка другого рецепта")
	}
	return tx.operation("lock", owner)
}

func (tx *serviceTransaction) UpdateRecipe(_ context.Context, owner, id int64, req SaveRequest) (Recipe, error) {
	if id != 11 {
		tx.t.Fatal("Замена другого рецепта")
	}
	tx.normalized = req
	return tx.result, tx.operation("update", owner)
}

func (tx *serviceTransaction) DeleteRecipe(_ context.Context, owner, id int64) error {
	if id != 11 {
		tx.t.Fatal("Удаление другого рецепта")
	}
	return tx.operation("delete", owner)
}

func serviceRequest() SaveRequest {
	quantity := "2.500"
	return SaveRequest{
		Title: "  Пирог  ", CookingTimeMinutes: 30, CategoryCode: "baking",
		Ingredients: []IngredientRequest{{Name: "  Мука  ", Quantity: &quantity, Unit: " г "}},
		Steps:       []StepRequest{{Text: "  Смешать\r\n\tИспечь  "}},
	}
}

func newServiceTransaction(t *testing.T) *serviceTransaction {
	return &serviceTransaction{t: t, result: Recipe{Summary: Summary{ID: 11, Title: "Пирог"}}}
}

func TestServiceValidationBeforeStorage(t *testing.T) {
	ctx := context.Background()
	service := NewService(nil) // Любое обращение к хранилищу приведёт к падению теста.
	if _, err := service.CreateRecipe(ctx, 7, "short", serviceRequest()); err == nil {
		t.Fatal("Принят некорректный ключ")
	}
	for name, change := range map[string]func(*SaveRequest){
		"title":       func(req *SaveRequest) { req.Title = " " },
		"time":        func(req *SaveRequest) { req.CookingTimeMinutes = 31 },
		"category":    func(req *SaveRequest) { req.CategoryCode = "unknown" },
		"ingredients": func(req *SaveRequest) { req.Ingredients = nil },
		"steps":       func(req *SaveRequest) { req.Steps = nil },
		"total": func(req *SaveRequest) {
			req.Steps = []StepRequest{
				{Text: strings.Repeat("я", 20000)}, {Text: strings.Repeat("я", 20000)},
				{Text: strings.Repeat("я", 10001)},
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			req := serviceRequest()
			change(&req)
			for _, save := range []func() (Recipe, error){
				func() (Recipe, error) { return service.CreateRecipe(ctx, 7, "service-recipe-0001", req) },
				func() (Recipe, error) { return service.UpdateRecipe(ctx, 7, 11, req) },
			} {
				got, err := save()
				var validation *ValidationError
				if !errors.As(err, &validation) || !reflect.DeepEqual(got, Recipe{}) {
					t.Fatalf("Ожидалась валидация без DTO: %+v, %v", got, err)
				}
			}
		})
	}
}

func TestServiceNormalizedSave(t *testing.T) {
	ctx := context.Background()
	for _, create := range []bool{true, false} {
		tx := newServiceTransaction(t)
		service := NewService(serviceStore{tx: tx})
		req := serviceRequest()
		before, _ := json.Marshal(req)
		started := time.Now().UTC()
		var got Recipe
		var err error
		if create {
			got, err = service.CreateRecipe(ctx, 7, "service-recipe-0001", req)
		} else {
			got, err = service.UpdateRecipe(ctx, 7, 11, req)
		}
		if err != nil || !reflect.DeepEqual(got, tx.result) {
			t.Fatalf("Сохранение: %+v, %v", got, err)
		}
		wantQuantity := "2.5"
		want := SaveRequest{
			Title: "Пирог", CookingTimeMinutes: 30, CategoryCode: "baking",
			Ingredients: []IngredientRequest{{Name: "Мука", Quantity: &wantQuantity, Unit: "г"}},
			Steps:       []StepRequest{{Text: "Смешать\n\tИспечь"}},
		}
		after, _ := json.Marshal(req)
		if !reflect.DeepEqual(tx.normalized, want) || string(before) != string(after) {
			t.Fatalf("Нормализация/изменение исходника: %+v", tx.normalized)
		}
		calls := []string{"lock", "update"}
		if create {
			calls = []string{"claim", "create", "save"}
			if !reflect.DeepEqual(tx.saved, got) || tx.expires.Before(started.Add(replayRetention)) ||
				tx.expires.After(time.Now().UTC().Add(replayRetention)) {
				t.Fatal("Неверный replay или срок хранения")
			}
		}
		if !reflect.DeepEqual(tx.calls, calls) {
			t.Fatalf("Порядок операций: %v", tx.calls)
		}
	}
}

func TestServiceWriteFailures(t *testing.T) {
	ctx := context.Background()
	sentinel := errors.New("storage failure with private recipe text")
	for operation, stages := range map[string][]string{
		"create": {"begin", "claim", "create", "save", "commit"},
		"update": {"begin", "lock", "update", "commit"},
		"delete": {"begin", "lock", "delete", "commit"},
	} {
		for _, stage := range stages {
			t.Run(operation+"/"+stage, func(t *testing.T) {
				tx := newServiceTransaction(t)
				tx.errAt, tx.err = stage, sentinel
				store := serviceStore{tx: tx}
				if stage == "begin" {
					store.beginErr = sentinel
				}
				if stage == "commit" {
					store.commitErr = sentinel
				}
				service := NewService(store)
				var got Recipe
				var err error
				switch operation {
				case "create":
					got, err = service.CreateRecipe(ctx, 7, "service-recipe-0001", serviceRequest())
				case "update":
					got, err = service.UpdateRecipe(ctx, 7, 11, serviceRequest())
				case "delete":
					err = service.DeleteRecipe(ctx, 7, 11)
				}
				if !errors.Is(err, sentinel) || !reflect.DeepEqual(got, Recipe{}) ||
					strings.Contains(err.Error(), "private") {
					t.Fatalf("Отказ выдан за успех: %+v, %v", got, err)
				}
			})
		}
	}
	tx := newServiceTransaction(t)
	if err := NewService(serviceStore{tx: tx}).DeleteRecipe(ctx, 7, 11); err != nil ||
		!reflect.DeepEqual(tx.calls, []string{"lock", "delete"}) {
		t.Fatalf("Удаление без предварительной блокировки: %v, %v", tx.calls, err)
	}
}

type failingRecipeReader struct {
	Store
	failure error
}

func (s failingRecipeReader) GetRecipes(context.Context, int64) ([]Summary, error) {
	return []Summary{{ID: 11}}, s.failure
}

func (s failingRecipeReader) GetRecipe(context.Context, int64, int64) (Recipe, error) {
	return Recipe{Summary: Summary{ID: 11}}, s.failure
}

func TestServiceReadFailures(t *testing.T) {
	ctx := context.Background()
	for _, failure := range []error{ErrNotFound, errors.New("private database diagnostic")} {
		s := NewService(failingRecipeReader{failure: failure})
		items, err := s.GetRecipes(ctx, 7)
		if !errors.Is(err, failure) || items != nil || strings.Contains(err.Error(), "private") {
			t.Fatalf("Ошибка коллекции подавлена или раскрывает данные: %v, %v", items, err)
		}
		got, err := s.GetRecipe(ctx, 7, 11)
		if !errors.Is(err, failure) || !reflect.DeepEqual(got, Recipe{}) || strings.Contains(err.Error(), "private") {
			t.Fatalf("Ошибка GET подавлена или раскрывает данные: %+v, %v", got, err)
		}
	}
}

func TestServiceReplay(t *testing.T) {
	ctx := context.Background()
	sentinel := errors.New("read failure")
	id := int64(11)
	original := Recipe{Summary: Summary{ID: id, Title: "Исходный"}}
	body, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name    string
		hash    string
		body    []byte
		readErr error
		wantErr error
	}{
		{name: "original", body: body},
		{name: "conflict", hash: strings.Repeat("0", 64), wantErr: ErrIdempotencyConflict},
		{name: "deleted", body: body, readErr: ErrNotFound, wantErr: ErrResourceDeleted},
		{name: "read failure", body: body, readErr: sentinel, wantErr: sentinel},
		{name: "corrupt", body: []byte(`{"id":"11","title":false}`)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tx := newServiceTransaction(t)
			tx.replay = Replay{Hash: tc.hash, ResourceID: &id, Response: tc.body}
			tx.errAt, tx.err = "get", tc.readErr
			got, err := NewService(serviceStore{tx: tx}).CreateRecipe(ctx, 7, "service-recipe-0001", serviceRequest())
			if tc.name == "original" {
				if err != nil || !reflect.DeepEqual(got, original) {
					t.Fatalf("Исходный ответ: %+v, %v", got, err)
				}
			} else if err == nil || !reflect.DeepEqual(got, Recipe{}) ||
				(tc.wantErr != nil && !errors.Is(err, tc.wantErr)) {
				t.Fatalf("Ошибка replay: %+v, %v", got, err)
			}
			calls := []string{"claim", "get"}
			if tc.name == "conflict" {
				calls = []string{"claim"}
			}
			if !reflect.DeepEqual(tx.calls, calls) {
				t.Fatalf("Replay изменяет рецепт или пропускает проверку доступа: %v", tx.calls)
			}
		})
	}
}
