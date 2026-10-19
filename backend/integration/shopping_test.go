//go:build integration

package integration_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ZM854/shopping-manager/backend/internal/auth"
	"github.com/ZM854/shopping-manager/backend/internal/middleware"
	"github.com/ZM854/shopping-manager/backend/internal/router"
	"github.com/ZM854/shopping-manager/backend/internal/shopping"
	"github.com/ZM854/shopping-manager/backend/internal/testutil/testdb"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

type shoppingFixture struct {
	pool          *pgxpool.Pool
	service       *shopping.Service
	router        *gin.Engine
	first, second int64
	tokens        map[int64]string
}

func newShoppingFixture(t *testing.T) *shoppingFixture {
	t.Helper()
	pool := testdb.New(t)
	testdb.ApplyMigrations(t, pool, 0, 5)
	f := &shoppingFixture{pool: pool, service: shopping.NewService(shopping.NewRepository(pool)), tokens: make(map[int64]string)}
	f.first = migrationUser(t, pool, "first@example.test")
	f.second = migrationUser(t, pool, "second@example.test")
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	tokens := auth.NewTokenService(log, nil, "shopping-test-access-secret", "shopping-test-refresh-secret", time.Hour, time.Hour)
	for _, user := range []int64{f.first, f.second} {
		pair, err := tokens.GenerateTokens(user)
		if err != nil {
			t.Fatal(err)
		}
		f.tokens[user] = pair.AccessToken
	}
	f.router = router.New(log, shopping.NewHandler(f.service, log), auth.NewAuthHandler(log, nil), middleware.NewAuthMiddleware(tokens), "http://client.example.test")
	return f
}
func (f *shoppingFixture) request(user int64, method, path, body, key string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, "/api"+path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if user != 0 {
		req.Header.Set("Authorization", "Bearer "+f.tokens[user])
	}
	if key != "" {
		req.Header.Set("Idempotency-Key", key)
	}
	w := httptest.NewRecorder()
	f.router.ServeHTTP(w, req)
	return w
}
func shoppingResponse[T any](t *testing.T, w *httptest.ResponseRecorder, status int) T {
	t.Helper()
	if w.Code != status {
		t.Fatalf("Status %d, want %d: %s", w.Code, status, w.Body.String())
	}
	var result T
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	return result
}
func shoppingStatus(t *testing.T, w *httptest.ResponseRecorder, status int) {
	t.Helper()
	if w.Code != status {
		t.Fatalf("Status %d, want %d: %s", w.Code, status, w.Body.String())
	}
	if status == 204 && w.Body.Len() != 0 {
		t.Fatal("204 has a body")
	}
}
func (f *shoppingFixture) list(t *testing.T, user int64, key string) shopping.List {
	t.Helper()
	return shoppingResponse[shopping.List](t, f.request(user, "POST", "/shopping-lists", `{"name":" Покупки "}`, key), 201)
}
func shoppingPath(list int64) string   { return fmt.Sprintf("/shopping-lists/%d", list) }
func itemPath(list, item int64) string { return fmt.Sprintf("%s/items/%d", shoppingPath(list), item) }
func (f *shoppingFixture) item(t *testing.T, user, list int64, key string) shopping.Item {
	t.Helper()
	return shoppingResponse[shopping.Item](t, f.request(user, "POST", shoppingPath(list)+"/items", `{"name":" Молоко 🥛 ","quantity":"2.675","unit":" Л ","isMarked":true}`, key), 201)
}

func TestShoppingHTTPCRUDIsolationAndBulkActions(t *testing.T) {
	t.Parallel()
	f := newShoppingFixture(t)
	lists := shoppingResponse[[]shopping.List](t, f.request(f.first, "GET", "/shopping-lists", "", ""), 200)
	if lists == nil || len(lists) != 0 {
		t.Fatal("New user has lists")
	}
	a, b, c := f.list(t, f.first, "create-list-000001"), f.list(t, f.first, "create-list-000002"), f.list(t, f.second, "create-list-000001")
	lists = shoppingResponse[[]shopping.List](t, f.request(f.first, "GET", "/shopping-lists", "", ""), 200)
	if len(lists) != 2 || lists[0].ID != a.ID || lists[1].ID != b.ID {
		t.Fatal("List order or isolation lost")
	}
	empty := shoppingResponse[[]shopping.Item](t, f.request(f.first, "GET", shoppingPath(a.ID)+"/items", "", ""), 200)
	if empty == nil || len(empty) != 0 {
		t.Fatal("Empty items must be []")
	}
	renamed := shoppingResponse[shopping.List](t, f.request(f.first, "PATCH", shoppingPath(a.ID), `{"name":" На неделю "}`, ""), 200)
	if renamed.Name != "На неделю" || !renamed.UpdatedAt.After(a.UpdatedAt) || !renamed.CreatedAt.Equal(a.CreatedAt) {
		t.Fatal("Rename/timestamps lost")
	}
	ia, ib, ic := f.item(t, f.first, a.ID, "create-item-000001"), f.item(t, f.first, b.ID, "create-item-000001"), f.item(t, f.second, c.ID, "create-item-000001")
	if ia.Name != "Молоко 🥛" || ia.Quantity == nil || *ia.Quantity != "2.675" || ia.Unit != "Л" || !ia.IsMarked {
		t.Fatalf("Normalization: %+v", ia)
	}
	read := shoppingResponse[shopping.Item](t, f.request(f.first, "GET", itemPath(a.ID, ia.ID), "", ""), 200)
	if !reflect.DeepEqual(read, ia) {
		t.Fatal("Read changed decimal or fields")
	}
	defaults := shoppingResponse[shopping.Item](t, f.request(f.first, "POST", shoppingPath(a.ID)+"/items", `{"name":"Хлеб"}`, "create-item-000002"), 201)
	if defaults.Quantity != nil || defaults.Unit != "" || defaults.IsMarked {
		t.Fatal("Defaults lost")
	}
	items := shoppingResponse[[]shopping.Item](t, f.request(f.first, "GET", shoppingPath(a.ID)+"/items", "", ""), 200)
	if len(items) != 2 || items[0].ID != ia.ID || items[1].ID != defaults.ID {
		t.Fatal("Item order lost")
	}
	patched := shoppingResponse[shopping.Item](t, f.request(f.first, "PATCH", itemPath(a.ID, ia.ID), `{"isMarked":false}`, ""), 200)
	if patched.IsMarked || patched.Name != ia.Name || patched.Unit != ia.Unit || *patched.Quantity != *ia.Quantity {
		t.Fatal("Mark overwrote other fields")
	}
	patched = shoppingResponse[shopping.Item](t, f.request(f.first, "PATCH", itemPath(a.ID, ia.ID), `{"quantity":null,"unit":"","name":"Кефир"}`, ""), 200)
	if patched.Quantity != nil || patched.Unit != "" || patched.Name != "Кефир" || patched.IsMarked {
		t.Fatal("Null/empty patch lost")
	}
	patched = shoppingResponse[shopping.Item](t, f.request(f.first, "PATCH", itemPath(a.ID, ia.ID), `{"quantity":"999999999.999"}`, ""), 200)
	if *patched.Quantity != "999999999.999" {
		t.Fatal("Maximum decimal rounded")
	}
	for _, marked := range []bool{true, false} {
		shoppingStatus(t, f.request(f.first, "PATCH", shoppingPath(a.ID)+"/items/mark-all", fmt.Sprintf(`{"isMarked":%t}`, marked), ""), 204)
		items = shoppingResponse[[]shopping.Item](t, f.request(f.first, "GET", shoppingPath(a.ID)+"/items", "", ""), 200)
		for _, item := range items {
			if item.IsMarked != marked {
				t.Fatal("Bulk mark missed item")
			}
		}
		untouched := shoppingResponse[shopping.Item](t, f.request(f.first, "GET", itemPath(b.ID, ib.ID), "", ""), 200)
		if !untouched.IsMarked {
			t.Fatal("Bulk mark affected another list")
		}
	}
	// Подстановка позиции из другого своего списка и чужого пользователя — одинаковый 404.
	for _, target := range []struct{ list, item int64 }{{b.ID, ia.ID}, {c.ID, ic.ID}, {a.ID, ic.ID}, {a.ID, 9223372036854775807}} {
		for _, method := range []string{"GET", "PATCH", "DELETE"} {
			shoppingStatus(t, f.request(f.first, method, itemPath(target.list, target.item), `{"name":"Подмена"}`, ""), 404)
		}
	}
	for _, method := range []string{"GET", "PATCH", "DELETE"} {
		shoppingStatus(t, f.request(f.first, method, shoppingPath(c.ID), `{"name":"Подмена"}`, ""), 404)
	}
	for _, target := range []int64{c.ID, 9223372036854775807} {
		for _, method := range []string{"GET", "POST", "DELETE"} {
			shoppingStatus(t, f.request(f.first, method, shoppingPath(target)+"/items", `{"name":"Подмена"}`, "foreign-item-00001"), 404)
		}
		shoppingStatus(t, f.request(f.first, "PATCH", shoppingPath(target)+"/items/mark-all", `{"isMarked":false}`, ""), 404)
	}
	shoppingStatus(t, f.request(f.first, "DELETE", itemPath(a.ID, defaults.ID), "", ""), 204)
	shoppingStatus(t, f.request(f.first, "DELETE", itemPath(a.ID, defaults.ID), "", ""), 404)
	for range 2 {
		shoppingStatus(t, f.request(f.first, "DELETE", shoppingPath(a.ID)+"/items", "", ""), 204)
		shoppingStatus(t, f.request(f.first, "PATCH", shoppingPath(a.ID)+"/items/mark-all", `{"isMarked":true}`, ""), 204)
	}
	if got := migrationScalar(t, f.pool, `SELECT count(*)::text FROM shopping_items`); got != "2" {
		t.Fatal("Clear affected other lists")
	}
	meta := shoppingResponse[shopping.List](t, f.request(f.first, "GET", shoppingPath(a.ID), "", ""), 200)
	if !meta.UpdatedAt.After(renamed.UpdatedAt) {
		t.Fatal("Contents did not touch parent")
	}
	shoppingStatus(t, f.request(f.first, "DELETE", shoppingPath(a.ID), "", ""), 204)
	shoppingStatus(t, f.request(f.first, "DELETE", shoppingPath(b.ID), "", ""), 204)
	shoppingStatus(t, f.request(f.first, "DELETE", shoppingPath(b.ID), "", ""), 404)
	lists = shoppingResponse[[]shopping.List](t, f.request(f.first, "GET", "/shopping-lists", "", ""), 200)
	if len(lists) != 0 {
		t.Fatal("Cannot delete last list")
	}
	if _, err := f.service.GetItem(context.Background(), f.second, c.ID, ic.ID); err != nil {
		t.Fatal("Deleted another user's items")
	}
}

func TestShoppingIdempotencyReplayScopeExpiryAndDeletion(t *testing.T) {
	t.Parallel()
	f := newShoppingFixture(t)
	a := f.list(t, f.first, "shared-action-0001")
	shoppingStatus(t, f.request(f.first, "PATCH", shoppingPath(a.ID), `{"name":"Новое имя"}`, ""), 200)
	replay := shoppingResponse[shopping.List](t, f.request(f.first, "POST", "/shopping-lists", `{"name":"Покупки"}`, "shared-action-0001"), 201)
	if !reflect.DeepEqual(a, replay) {
		t.Fatal("List replay must return original DTO after rename")
	}
	conflict := shoppingResponse[shopping.ErrorResponse](t, f.request(f.first, "POST", "/shopping-lists", `{"name":"Другое"}`, "shared-action-0001"), 409)
	if conflict.Code != "idempotency_conflict" {
		t.Fatal(conflict)
	}
	b := f.list(t, f.first, "shared-action-0002")
	f.list(t, f.second, "shared-action-0001")
	item := f.item(t, f.first, a.ID, "shared-action-0001")
	f.item(t, f.first, b.ID, "shared-action-0001")
	shoppingStatus(t, f.request(f.first, "PATCH", itemPath(a.ID, item.ID), `{"name":"Изменено","quantity":null}`, ""), 200)
	itemReplay := shoppingResponse[shopping.Item](t, f.request(f.first, "POST", shoppingPath(a.ID)+"/items", `{"name":"Молоко 🥛","quantity":"2.675","unit":"Л","isMarked":true}`, "shared-action-0001"), 201)
	if !reflect.DeepEqual(item, itemReplay) {
		t.Fatal("Item replay must return original DTO after edits")
	}
	shoppingStatus(t, f.request(f.first, "POST", shoppingPath(a.ID)+"/items", `{"name":"Другое"}`, "shared-action-0001"), 409)
	normalized := shoppingResponse[shopping.Item](t, f.request(f.first, "POST", shoppingPath(a.ID)+"/items", `{"name":"А","quantity":"1.000"}`, "normalized-item-01"), 201)
	normReplay := shoppingResponse[shopping.Item](t, f.request(f.first, "POST", shoppingPath(a.ID)+"/items", `{"name":" А ","quantity":"1","unit":" ","isMarked":false}`, "normalized-item-01"), 201)
	if !reflect.DeepEqual(normalized, normReplay) {
		t.Fatal("Canonical default/decimal hash differs")
	}
	if got := migrationScalar(t, f.pool, `SELECT bool_and(expires_at > statement_timestamp() + interval '23 hours')::text FROM shopping_idempotency`); got != "true" {
		t.Fatal("Replay retention too short")
	}
	shoppingStatus(t, f.request(f.first, "DELETE", itemPath(a.ID, item.ID), "", ""), 204)
	deleted := shoppingResponse[shopping.ErrorResponse](t, f.request(f.first, "POST", shoppingPath(a.ID)+"/items", `{"name":"Молоко 🥛","quantity":"2.675","unit":"Л","isMarked":true}`, "shared-action-0001"), 409)
	if deleted.Code != "resource_deleted" {
		t.Fatal(deleted)
	}
	shoppingStatus(t, f.request(f.first, "DELETE", shoppingPath(a.ID), "", ""), 204)
	deleted = shoppingResponse[shopping.ErrorResponse](t, f.request(f.first, "POST", "/shopping-lists", `{"name":"Покупки"}`, "shared-action-0001"), 409)
	if deleted.Code != "resource_deleted" {
		t.Fatal(deleted)
	}
	shoppingStatus(t, f.request(f.first, "POST", shoppingPath(a.ID)+"/items", `{"name":"Молоко 🥛","quantity":"2.675","unit":"Л","isMarked":true}`, "shared-action-0001"), 404)
	migrationExec(t, f.pool, `UPDATE shopping_idempotency SET expires_at = now() - interval '1 second' WHERE owner_id = $1 AND operation = 'lists' AND key = 'shared-action-0001'`, f.first)
	renewed := shoppingResponse[shopping.List](t, f.request(f.first, "POST", "/shopping-lists", `{"name":"После срока"}`, "shared-action-0001"), 201)
	if renewed.ID == a.ID || renewed.Name != "После срока" {
		t.Fatal("Expired replay not replaced")
	}
	migrationExec(t, f.pool, `UPDATE shopping_idempotency SET expires_at = now() - interval '1 second' WHERE owner_id = $1 AND operation = $2 AND key = 'shared-action-0001'`, f.first, fmt.Sprintf("items:%d", b.ID))
	renewedItem := shoppingResponse[shopping.Item](t, f.request(f.first, "POST", shoppingPath(b.ID)+"/items", `{"name":"После срока"}`, "shared-action-0001"), 201)
	if renewedItem.Name != "После срока" {
		t.Fatal("Expired item replay not replaced")
	}
}

func TestShoppingConcurrentPOSTsCreateOneResource(t *testing.T) {
	t.Parallel()
	f := newShoppingFixture(t)
	for _, target := range []string{"lists", "items"} {
		t.Run(target, func(t *testing.T) {
			path, body := "/shopping-lists", `{"name":"Одновременно"}`
			if target == "items" {
				list := f.list(t, f.first, "concurrent-parent")
				path = shoppingPath(list.ID) + "/items"
			}
			const count = 12
			start := make(chan struct{})
			results := make(chan *httptest.ResponseRecorder, count)
			var wg sync.WaitGroup
			for range count {
				wg.Go(func() { <-start; results <- f.request(f.first, "POST", path, body, "parallel-action-01") })
			}
			close(start)
			wg.Wait()
			close(results)
			original := ""
			for result := range results {
				shoppingStatus(t, result, 201)
				var obj map[string]json.RawMessage
				if err := json.Unmarshal(result.Body.Bytes(), &obj); err != nil {
					t.Fatal(err)
				}
				canonical, err := json.Marshal(obj)
				if err != nil {
					t.Fatal(err)
				}
				if original == "" {
					original = string(canonical)
				} else if original != string(canonical) {
					t.Fatal("Concurrent POST returned another resource")
				}
			}
			if target == "lists" {
				if got := migrationScalar(t, f.pool, `SELECT count(*)::text FROM shopping_lists WHERE name = 'Одновременно'`); got != "1" {
					t.Fatal("Duplicate lists")
				}
			} else if got := migrationScalar(t, f.pool, `SELECT count(*)::text FROM shopping_items`); got != "1" {
				t.Fatal("Duplicate items")
			}
		})
	}
}

func TestShoppingMutationsRollbackOnParentAndReplayFailure(t *testing.T) {
	t.Parallel()
	f := newShoppingFixture(t)
	ctx := context.Background()
	list := f.list(t, f.first, "rollback-list-001")
	item := f.item(t, f.first, list.ID, "rollback-item-001")
	original := migrationScalar(t, f.pool, `SELECT jsonb_agg(to_jsonb(shopping_items))::text FROM shopping_items`)
	timestamp := migrationScalar(t, f.pool, `SELECT updated_at::text FROM shopping_lists WHERE id = $1`, list.ID)
	migrationExec(t, f.pool, `CREATE FUNCTION fail_shopping_write() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'injected write failure'; END $$`)
	migrationExec(t, f.pool, `CREATE TRIGGER fail_touch BEFORE UPDATE ON shopping_lists FOR EACH ROW EXECUTE FUNCTION fail_shopping_write()`)
	yes := true
	operations := []func() error{
		func() error {
			_, err := f.service.CreateItem(ctx, f.first, list.ID, "rollback-new-0001", shopping.CreateItemRequest{Name: "Откат"})
			return err
		},
		func() error {
			_, err := f.service.UpdateItem(ctx, f.first, list.ID, item.ID, shopping.UpdateItemRequest{Name: shopping.PatchField[string]{Present: true, Value: shoppingPtr("Откат")}})
			return err
		},
		func() error { return f.service.DeleteItem(ctx, f.first, list.ID, item.ID) },
		func() error { return f.service.ClearItems(ctx, f.first, list.ID) },
		func() error {
			return f.service.MarkAll(ctx, f.first, list.ID, shopping.MarkAllRequest{IsMarked: shopping.PatchField[bool]{Present: true, Value: &yes}})
		},
	}
	for i, op := range operations {
		if err := op(); err == nil {
			t.Fatalf("Operation %d did not propagate failure", i)
		}
		if got := migrationScalar(t, f.pool, `SELECT jsonb_agg(to_jsonb(shopping_items))::text FROM shopping_items`); got != original {
			t.Fatalf("Operation %d partially committed", i)
		}
		if got := migrationScalar(t, f.pool, `SELECT updated_at::text FROM shopping_lists WHERE id = $1`, list.ID); got != timestamp {
			t.Fatal("Timestamp committed on failure")
		}
	}
	if got := migrationScalar(t, f.pool, `SELECT count(*)::text FROM shopping_idempotency WHERE key = 'rollback-new-0001'`); got != "0" {
		t.Fatal("Failed item left a replay key")
	}
	migrationExec(t, f.pool, `DROP TRIGGER fail_touch ON shopping_lists`)
	migrationExec(t, f.pool, `CREATE TRIGGER fail_replay BEFORE UPDATE ON shopping_idempotency FOR EACH ROW WHEN (NEW.response IS NOT NULL) EXECUTE FUNCTION fail_shopping_write()`)
	for _, itemMode := range []bool{false, true} {
		if itemMode {
			_, err := f.service.CreateItem(ctx, f.first, list.ID, "replay-failure-01", shopping.CreateItemRequest{Name: "Откат"})
			if err == nil {
				t.Fatal("Replay save failure swallowed")
			}
		} else {
			_, err := f.service.CreateList(ctx, f.first, "replay-failure-01", shopping.CreateListRequest{Name: "Откат"})
			if err == nil {
				t.Fatal("Replay save failure swallowed")
			}
		}
	}
	if got := migrationScalar(t, f.pool, `SELECT count(*)::text FROM shopping_lists`); got != "1" {
		t.Fatal("Failed replay left list")
	}
	if got := migrationScalar(t, f.pool, `SELECT jsonb_agg(to_jsonb(shopping_items))::text FROM shopping_items`); got != original {
		t.Fatal("Failed replay left item")
	}
	if got := migrationScalar(t, f.pool, `SELECT updated_at::text FROM shopping_lists WHERE id = $1`, list.ID); got != timestamp {
		t.Fatal("Failed replay left timestamp")
	}
	if got := migrationScalar(t, f.pool, `SELECT count(*)::text FROM shopping_idempotency WHERE key = 'replay-failure-01'`); got != "0" {
		t.Fatal("Failed replay left key")
	}
	migrationExec(t, f.pool, `DROP TRIGGER fail_replay ON shopping_idempotency`)
	if _, err := f.service.CreateList(ctx, f.first, "replay-failure-01", shopping.CreateListRequest{Name: "Повтор"}); err != nil {
		t.Fatal("Retry after rollback failed", err)
	}
}
func shoppingPtr[T any](v T) *T { return &v }

func TestShoppingUpgradeMakesLegacyItemsAvailable(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	testdb.ApplyMigrations(t, pool, 0, 3)
	user := migrationUser(t, pool, "upgrade@example.test")
	migrationExec(t, pool, `INSERT INTO products (user_id,name,quantity,unit,is_marked) VALUES ($1,'Молоко',0.1,'л',true)`, user)
	testdb.ApplyMigrations(t, pool, 3, 5)
	service := shopping.NewService(shopping.NewRepository(pool))
	lists, err := service.GetLists(context.Background(), user)
	if err != nil || len(lists) != 1 {
		t.Fatalf("Upgraded lists: %v %v", lists, err)
	}
	items, err := service.GetItems(context.Background(), user, lists[0].ID)
	if err != nil || len(items) != 1 || items[0].Quantity == nil || *items[0].Quantity != "0.1" || !items[0].IsMarked {
		t.Fatalf("Upgraded items: %v %v", items, err)
	}
}

func TestShoppingConnectionAndCancellationFailures(t *testing.T) {
	f := newShoppingFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := f.service.CreateList(ctx, f.first, "cancelled-list-01", shopping.CreateListRequest{Name: "A"}); err == nil {
		t.Fatal("Canceled create succeeded")
	}
	f.pool.Close()
	operations := []func() error{
		func() error { _, err := f.service.GetLists(context.Background(), f.first); return err },
		func() error { _, err := f.service.GetList(context.Background(), f.first, 1); return err },
		func() error { _, err := f.service.GetItems(context.Background(), f.first, 1); return err },
		func() error { _, err := f.service.GetItem(context.Background(), f.first, 1, 1); return err },
		func() error {
			_, err := f.service.CreateList(context.Background(), f.first, "closed-pool-00001", shopping.CreateListRequest{Name: "A"})
			return err
		},
	}
	for _, op := range operations {
		if err := op(); err == nil || errors.Is(err, shopping.ErrNotFound) {
			t.Fatal("Infrastructure failure hidden as success/not-found")
		}
	}
}

func TestShoppingRepositoryEnforcesOwnershipWithoutService(t *testing.T) {
	t.Parallel()
	f := newShoppingFixture(t)
	list := f.list(t, f.first, "raw-repo-list-001")
	item := f.item(t, f.first, list.ID, "raw-repo-item-001")
	repo := shopping.NewRepository(f.pool)
	ctx := context.Background()
	before := migrationScalar(t, f.pool, `SELECT jsonb_build_array((SELECT jsonb_agg(to_jsonb(l)) FROM shopping_lists l), (SELECT jsonb_agg(to_jsonb(i)) FROM shopping_items i))::text`)
	operations := []func() error{
		func() error { _, err := repo.GetList(ctx, f.second, list.ID); return err },
		func() error { _, err := repo.GetItems(ctx, f.second, list.ID); return err },
		func() error { _, err := repo.GetItem(ctx, f.second, list.ID, item.ID); return err },
		func() error { _, err := repo.UpdateList(ctx, f.second, list.ID, "Подмена"); return err },
		func() error { return repo.DeleteList(ctx, f.second, list.ID) },
		func() error { return repo.LockList(ctx, f.second, list.ID) },
		func() error { return repo.TouchList(ctx, f.second, list.ID) },
		func() error {
			_, err := repo.CreateItem(ctx, f.second, list.ID, shopping.CreateItemRequest{Name: "Подмена"})
			return err
		},
		func() error {
			_, err := repo.UpdateItem(ctx, f.second, list.ID, item.ID, shopping.UpdateItemRequest{Name: shopping.PatchField[string]{Present: true, Value: shoppingPtr("Подмена")}})
			return err
		},
		func() error { return repo.DeleteItem(ctx, f.second, list.ID, item.ID) },
	}
	for i, op := range operations {
		if err := op(); !errors.Is(err, shopping.ErrNotFound) {
			t.Fatalf("Foreign SQL operation %d: %v", i, err)
		}
	}
	if err := repo.ClearItems(ctx, f.second, list.ID); err != nil {
		t.Fatal(err)
	}
	if err := repo.MarkAll(ctx, f.second, list.ID, false); err != nil {
		t.Fatal(err)
	}
	after := migrationScalar(t, f.pool, `SELECT jsonb_build_array((SELECT jsonb_agg(to_jsonb(l)) FROM shopping_lists l), (SELECT jsonb_agg(to_jsonb(i)) FROM shopping_items i))::text`)
	if before != after {
		t.Fatal("Repository changed another owner's data")
	}
	// Ошибка Commit тоже не должна оставить созданный ресурс.
	canceled, cancel := context.WithCancel(ctx)
	err := repo.InTx(canceled, func(tx shopping.Transaction) error {
		if _, err := tx.CreateList(canceled, f.first, shopping.CreateListRequest{Name: "Откат commit"}); err != nil {
			return err
		}
		cancel()
		return nil
	})
	if err == nil {
		t.Fatal("Canceled commit succeeded")
	}
	if got := migrationScalar(t, f.pool, `SELECT count(*)::text FROM shopping_lists WHERE name = 'Откат commit'`); got != "0" {
		t.Fatal("Canceled commit persisted resource")
	}
}

func TestShoppingIdempotencySchemaConstraintsAndOwnerCascade(t *testing.T) {
	t.Parallel()
	f := newShoppingFixture(t)
	f.list(t, f.first, "schema-first-0001")
	f.list(t, f.second, "schema-second-001")
	for _, tc := range []struct{ column, value string }{
		{"key", "short"}, {"key", "invalid action 0001"}, {"operation", "other"}, {"request_hash", "not-a-sha256"},
	} {
		_, err := f.pool.Exec(context.Background(), "UPDATE shopping_idempotency SET "+tc.column+" = $1 WHERE owner_id = $2", tc.value, f.first)
		if err == nil {
			t.Fatalf("Invalid %s accepted", tc.column)
		}
	}
	for _, sql := range []string{
		`UPDATE shopping_idempotency SET resource_id = NULL WHERE owner_id = $1`,
		`UPDATE shopping_idempotency SET resource_id = -1 WHERE owner_id = $1`,
		`UPDATE shopping_idempotency SET response = NULL WHERE owner_id = $1`,
		`UPDATE shopping_idempotency SET expires_at = NULL WHERE owner_id = $1`,
		`UPDATE shopping_idempotency SET owner_id = 9223372036854775807 WHERE owner_id = $1`,
	} {
		if _, err := f.pool.Exec(context.Background(), sql, f.first); err == nil {
			t.Fatal("Invalid replay row accepted")
		}
	}
	migrationExec(t, f.pool, `DELETE FROM users WHERE id = $1`, f.first)
	if got := migrationScalar(t, f.pool, `SELECT count(*)::text FROM shopping_idempotency WHERE owner_id = $1`, f.first); got != "0" {
		t.Fatal("Owner cascade left replay data")
	}
	if got := migrationScalar(t, f.pool, `SELECT count(*)::text FROM shopping_idempotency WHERE owner_id = $1`, f.second); got != "1" {
		t.Fatal("Owner cascade deleted another user's replay")
	}
}
