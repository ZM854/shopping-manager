//go:build integration

package integration_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ZM854/shopping-manager/backend/internal/shopping"
	"github.com/ZM854/shopping-manager/backend/internal/testutil/testdb"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type sharingFixture struct {
	pool              *pgxpool.Pool
	service           *shopping.Service
	author, recipient int64
}

func newSharingFixture(t *testing.T) *sharingFixture {
	t.Helper()
	pool := testdb.New(t)
	testdb.ApplyMigrations(t, pool, 0, 9)
	return &sharingFixture{
		pool: pool, service: shopping.NewService(shopping.NewRepository(pool)),
		author:    migrationUser(t, pool, "share-author@example.test"),
		recipient: migrationUser(t, pool, "share-recipient@example.test"),
	}
}

func sharingToken(value byte) string {
	return base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{value}, 32))
}

func (f *sharingFixture) list(t *testing.T) shopping.List {
	t.Helper()
	list, err := f.service.CreateList(context.Background(), f.author, "sharing-source-list-0001",
		shopping.CreateListRequest{Name: "Покупки 🥛"})
	if err != nil {
		t.Fatal(err)
	}
	return list
}

func (f *sharingFixture) items(t *testing.T, list shopping.List) []shopping.Item {
	t.Helper()
	var items []shopping.Item
	for i, req := range []shopping.CreateItemRequest{
		{Name: "Молоко 🥛", Quantity: shoppingPtr("2.675"), Unit: "л", IsMarked: true},
		{Name: "Второй", Quantity: nil, Unit: "", IsMarked: false},
		{Name: "Повтор", Quantity: shoppingPtr("0.001"), Unit: "кг", IsMarked: true},
		{Name: "Повтор", Quantity: shoppingPtr("999999999.999"), Unit: "шт", IsMarked: false},
	} {
		item, err := f.service.CreateItem(context.Background(), f.author, list.ID,
			fmt.Sprintf("sharing-source-item-%04d", i), req)
		if err != nil {
			t.Fatal(err)
		}
		items = append(items, item)
	}
	return items
}

func (f *sharingFixture) share(t *testing.T, list shopping.List, token byte, preserve bool) shopping.Share {
	t.Helper()
	share, err := f.service.CreateShare(context.Background(), f.author, list.ID,
		fmt.Sprintf("sharing-create-action-%04d", token),
		shopping.CreateShareRequest{Token: sharingToken(token), PreserveMarks: &preserve})
	if err != nil {
		t.Fatal(err)
	}
	return share
}

func (f *sharingFixture) importList(t *testing.T, user int64, token byte, key string) shopping.List {
	t.Helper()
	list, err := f.service.ImportShare(context.Background(), user, key,
		shopping.ImportShareRequest{Token: sharingToken(token)})
	if err != nil {
		t.Fatal(err)
	}
	return list
}

func assertSharingItems(t *testing.T, f *sharingFixture, user int64, list shopping.List, original []shopping.Item, preserve bool) {
	t.Helper()
	items, err := f.service.GetItems(context.Background(), user, list.ID)
	if err != nil || len(items) != len(original) || items == nil {
		t.Fatal("Импортированы не все позиции или неверная пустая коллекция", err)
	}
	for i, item := range items {
		want := original[i]
		if item.ID == want.ID || item.ListID != list.ID || item.ID <= 0 ||
			item.Name != want.Name || item.Unit != want.Unit || !reflect.DeepEqual(item.Quantity, want.Quantity) ||
			item.IsMarked != (preserve && want.IsMarked) || (i > 0 && item.ID <= items[i-1].ID) {
			t.Fatalf("Изменились значения, отметки, порядок или независимые ID позиции %d", i)
		}
	}
}

func TestSharingSnapshotsAndCopiesStayIndependent(t *testing.T) {
	t.Parallel()
	for _, preserve := range []bool{false, true} {
		t.Run(fmt.Sprintf("preserve=%t", preserve), func(t *testing.T) {
			f := newSharingFixture(t)
			ctx := context.Background()
			migrationExec(t, f.pool, `SELECT setval('shopping_items_id_seq', 9007199254740993)`)
			list := f.list(t)
			original := f.items(t, list)
			before, err := f.service.GetList(ctx, f.author, list.ID)
			if err != nil {
				t.Fatal(err)
			}
			f.share(t, list, 1, preserve)
			after, err := f.service.GetList(ctx, f.author, list.ID)
			if err != nil || !reflect.DeepEqual(before, after) {
				t.Fatal("Создание снимка изменило исходный список или updatedAt", err)
			}
			afterItems, err := f.service.GetItems(ctx, f.author, list.ID)
			if err != nil || !reflect.DeepEqual(original, afterItems) {
				t.Fatal("Создание снимка изменило позиции или отметки оригинала", err)
			}
			if _, err := f.service.UpdateList(ctx, f.author, list.ID,
				shopping.UpdateListRequest{Name: shopping.PatchField[string]{Present: true, Value: shoppingPtr("Новое имя")}}); err != nil {
				t.Fatal(err)
			}
			if _, err := f.service.UpdateItem(ctx, f.author, list.ID, original[0].ID,
				shopping.UpdateItemRequest{Name: shopping.PatchField[string]{Present: true, Value: shoppingPtr("Изменено")}}); err != nil {
				t.Fatal(err)
			}
			if err := f.service.MarkAll(ctx, f.author, list.ID,
				shopping.MarkAllRequest{IsMarked: shopping.PatchField[bool]{Present: true, Value: shoppingPtr(false)}}); err != nil {
				t.Fatal(err)
			}
			if err := f.service.ClearItems(ctx, f.author, list.ID); err != nil {
				t.Fatal(err)
			}
			if err := f.service.DeleteList(ctx, f.author, list.ID); err != nil {
				t.Fatal(err)
			}
			copy := f.importList(t, f.recipient, 1, "sharing-import-action-0001")
			other := f.importList(t, f.recipient, 1, "sharing-import-action-0002")
			self := f.importList(t, f.author, 1, "sharing-import-action-0001")
			for _, result := range []shopping.List{copy, other, self} {
				if result.ID == list.ID || result.Name != list.Name || result.CreatedAt.IsZero() ||
					!result.CreatedAt.Equal(result.UpdatedAt) || !result.CreatedAt.After(list.CreatedAt) {
					t.Fatal("Копия потеряла имя снимка, новые ID или собственные даты")
				}
			}
			assertSharingItems(t, f, f.recipient, copy, original, preserve)
			assertSharingItems(t, f, f.recipient, other, original, preserve)
			assertSharingItems(t, f, f.author, self, original, preserve)
			if _, err := f.service.GetList(ctx, f.author, copy.ID); !errors.Is(err, shopping.ErrNotFound) {
				t.Fatal("Автор получил доступ к чужой копии", err)
			}
			_, err = f.service.UpdateList(ctx, f.author, copy.ID,
				shopping.UpdateListRequest{Name: shopping.PatchField[string]{Present: true, Value: shoppingPtr("Чужое")}})
			if !errors.Is(err, shopping.ErrNotFound) {
				t.Fatal("Автор изменил чужую копию", err)
			}
			if err := f.service.ClearItems(ctx, f.recipient, copy.ID); err != nil {
				t.Fatal(err)
			}
			assertSharingItems(t, f, f.recipient, other, original, preserve)
			assertSharingItems(t, f, f.author, self, original, preserve)
		})
	}
}

func TestSharingEmptySnapshotAndSecretStorage(t *testing.T) {
	t.Parallel()
	f := newSharingFixture(t)
	list := f.list(t)
	f.share(t, list, 1, false)
	copy := f.importList(t, f.recipient, 1, "sharing-import-empty-0001")
	assertSharingItems(t, f, f.recipient, copy, nil, false)
	wantHash := sha256.Sum256(bytes.Repeat([]byte{1}, 32))
	gotHash := migrationScalar(t, f.pool, `SELECT encode(token_hash, 'hex') FROM shopping_list_shares`)
	if gotHash != hex.EncodeToString(wantHash[:]) {
		t.Fatal("В БД сохранён не SHA-256 декодированного секрета")
	}
	data := migrationScalar(t, f.pool, `SELECT jsonb_build_object(
		'shares', (SELECT jsonb_agg(to_jsonb(s)) FROM shopping_list_shares s),
		'replays', (SELECT jsonb_agg(to_jsonb(r)) FROM shopping_idempotency r)
	)::text`)
	if strings.Contains(data, sharingToken(1)) || strings.Contains(data, "/share#") {
		t.Fatal("Открытый секрет или URL сохранён в снимке или исходном ответе")
	}
	if migrationScalar(t, f.pool, `
		SELECT bool_and(expires_at > clock_timestamp() + interval '23 hours')::text
		FROM shopping_idempotency WHERE operation = 'share-imports' OR operation LIKE 'shares:%'`) != "true" {
		t.Fatal("Новые операции не обеспечивают хранение исходного ответа минимум 24 часа")
	}
}

func TestSharingAccessAndMissingResources(t *testing.T) {
	t.Parallel()
	f := newSharingFixture(t)
	ctx := context.Background()
	list := f.list(t)
	req := shopping.CreateShareRequest{Token: sharingToken(1), PreserveMarks: shoppingPtr(false)}
	for _, source := range []int64{list.ID, 9223372036854775807} {
		if _, err := f.service.CreateShare(ctx, f.recipient, source, "sharing-forbidden-0001", req); !errors.Is(err, shopping.ErrNotFound) {
			t.Fatal("Получен снимок чужого или отсутствующего списка", err)
		}
	}
	if _, err := f.service.ImportShare(ctx, f.recipient, "sharing-unknown-0001",
		shopping.ImportShareRequest{Token: sharingToken(2)}); !errors.Is(err, shopping.ErrNotFound) {
		t.Fatal("Неизвестный секрет принят", err)
	}
	if migrationScalar(t, f.pool, `SELECT count(*)::text FROM shopping_idempotency WHERE operation <> 'lists'`) != "0" {
		t.Fatal("Отказ доступа оставил незавершённые записи идемпотентности")
	}
	share := f.share(t, list, 1, false)
	if _, err := shopping.NewRepository(f.pool).GetShare(ctx, f.recipient, share.ID); !errors.Is(err, shopping.ErrNotFound) {
		t.Fatal("SQL получения метаданных снимка не проверяет автора", err)
	}
}

func TestSharingRepositoryChecksSourceAndTargetOwners(t *testing.T) {
	t.Parallel()
	f := newSharingFixture(t)
	ctx := context.Background()
	list := f.list(t)
	f.items(t, list)
	share := f.share(t, list, 1, true)
	copy := f.importList(t, f.recipient, 1, "sharing-repository-copy-0001")
	other, err := f.service.CreateList(ctx, f.author, "sharing-repository-list-0001",
		shopping.CreateListRequest{Name: "Другой источник"})
	if err != nil {
		t.Fatal(err)
	}
	otherShare := f.share(t, other, 2, true)
	repo := shopping.NewRepository(f.pool)
	before := migrationScalar(t, f.pool, sharingDataState)
	err = repo.InTx(ctx, func(tx shopping.Transaction) error {
		if err := tx.CopyListToShare(ctx, f.recipient, list.ID, otherShare.ID); err != nil {
			return err
		}
		if err := tx.CopyListToShare(ctx, f.author, list.ID, otherShare.ID); err != nil {
			return err
		}
		return tx.CopyShareToList(ctx, f.author, copy.ID, share.ID)
	})
	if err != nil || migrationScalar(t, f.pool, sharingDataState) != before {
		t.Fatal("SQL копирования смешал владельцев, исходники или назначения", err)
	}
	if _, err := repo.CreateShare(ctx, f.recipient, list.ID, make([]byte, 32), false); !errors.Is(err, shopping.ErrNotFound) {
		t.Fatal("SQL создания снимка не проверяет владельца оригинала", err)
	}
}

func TestSharingImportDoesNotLockSourceOrSerializeRecipients(t *testing.T) {
	t.Parallel()
	f := newSharingFixture(t)
	list := f.list(t)
	original := f.items(t, list)
	f.share(t, list, 1, true)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var copy shopping.List
	err := shopping.NewRepository(f.pool).InTx(ctx, func(tx shopping.Transaction) error {
		if err := tx.LockList(ctx, f.author, list.ID); err != nil {
			return err
		}
		hash := sha256.Sum256(bytes.Repeat([]byte{1}, 32))
		if _, _, err := tx.FindShare(ctx, hash[:]); err != nil {
			return err
		}
		// Другая транзакция завершается, пока источник и чтение снимка удерживаются первой.
		var err error
		copy, err = f.service.ImportShare(ctx, f.recipient, "sharing-independent-read-0001",
			shopping.ImportShareRequest{Token: sharingToken(1)})
		return err
	})
	if err != nil {
		t.Fatal("Импорт зависит от блокировки оригинала или другого читателя снимка", err)
	}
	assertSharingItems(t, f, f.recipient, copy, original, true)
}

func TestSharingReplayNewActionsAndDeletedCopy(t *testing.T) {
	t.Parallel()
	f := newSharingFixture(t)
	ctx := context.Background()
	list := f.list(t)
	originalItems := f.items(t, list)
	share := f.share(t, list, 1, true)
	createKey := "sharing-create-action-0001"
	req := shopping.CreateShareRequest{Token: sharingToken(1), PreserveMarks: shoppingPtr(true)}
	if err := f.service.DeleteList(ctx, f.author, list.ID); err != nil {
		t.Fatal(err)
	}
	replayed, err := f.service.CreateShare(ctx, f.author, list.ID, createKey, req)
	if err != nil || !reflect.DeepEqual(replayed, share) {
		t.Fatal("Повтор снимка после удаления оригинала не вернул исходный DTO", err)
	}
	for _, changed := range []shopping.CreateShareRequest{
		{Token: sharingToken(1), PreserveMarks: shoppingPtr(false)},
		{Token: sharingToken(2), PreserveMarks: shoppingPtr(true)},
	} {
		if _, err := f.service.CreateShare(ctx, f.author, list.ID, createKey, changed); !errors.Is(err, shopping.ErrIdempotencyConflict) {
			t.Fatal("Изменённое тело повтора снимка принято", err)
		}
	}
	key := "sharing-import-replay-0001"
	copy := f.importList(t, f.recipient, 1, key)
	other := f.importList(t, f.recipient, 1, "sharing-import-replay-0002")
	if copy.ID == other.ID {
		t.Fatal("Новое подтверждение не создало отдельную копию")
	}
	if _, err := f.service.UpdateList(ctx, f.recipient, copy.ID,
		shopping.UpdateListRequest{Name: shopping.PatchField[string]{Present: true, Value: shoppingPtr("Мои правки")}}); err != nil {
		t.Fatal(err)
	}
	if err := f.service.ClearItems(ctx, f.recipient, copy.ID); err != nil {
		t.Fatal(err)
	}
	copyReplay := f.importList(t, f.recipient, 1, key)
	if !reflect.DeepEqual(copy, copyReplay) {
		t.Fatal("Повтор импорта изменил исходный DTO")
	}
	current, err := f.service.GetList(ctx, f.recipient, copy.ID)
	if err != nil || current.Name != "Мои правки" ||
		migrationScalar(t, f.pool, `SELECT count(*)::text FROM shopping_items WHERE list_id = $1`, copy.ID) != "0" {
		t.Fatal("Повтор импорта перезаписал правки получателя", err)
	}
	assertSharingItems(t, f, f.recipient, other, originalItems, true)
	if _, err := f.service.ImportShare(ctx, f.recipient, key,
		shopping.ImportShareRequest{Token: sharingToken(2)}); !errors.Is(err, shopping.ErrIdempotencyConflict) {
		t.Fatal("Тот же ключ импорта принят с другим секретом", err)
	}
	if err := f.service.DeleteList(ctx, f.recipient, copy.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.ImportShare(ctx, f.recipient, key,
		shopping.ImportShareRequest{Token: sharingToken(1)}); !errors.Is(err, shopping.ErrResourceDeleted) {
		t.Fatal("Прежний ключ не обнаружил удалённый результат", err)
	}
	newCopy := f.importList(t, f.recipient, 1, "sharing-import-replay-0003")
	assertSharingItems(t, f, f.recipient, newCopy, originalItems, true)
	// Replay проверяет владельца списка, даже если ID в сохранённом ответе остался прежним.
	migrationExec(t, f.pool, `UPDATE shopping_lists SET owner_id = $1 WHERE id = $2`, f.author, newCopy.ID)
	if _, err := f.service.ImportShare(ctx, f.recipient, "sharing-import-replay-0003",
		shopping.ImportShareRequest{Token: sharingToken(1)}); !errors.Is(err, shopping.ErrResourceDeleted) {
		t.Fatal("Повтор вернул результат без проверки текущего владельца", err)
	}
	// Готовая личная копия и её replay не зависят от дальнейшего существования снимка.
	migrationExec(t, f.pool, `DELETE FROM shopping_list_shares WHERE id = $1`, share.ID)
	if !reflect.DeepEqual(f.importList(t, f.recipient, 1, "sharing-import-replay-0002"), other) {
		t.Fatal("Удаление снимка нарушило повтор уже завершённого импорта")
	}
	if _, err := f.service.CreateShare(ctx, f.author, list.ID, createKey, req); !errors.Is(err, shopping.ErrResourceDeleted) {
		t.Fatal("Прежний ключ не обнаружил удалённый снимок", err)
	}
}

func TestSharingExpiredKeysAndOperationScopes(t *testing.T) {
	t.Parallel()
	f := newSharingFixture(t)
	list := f.list(t)
	ctx := context.Background()
	key := "sharing-same-action-0001"
	req := shopping.CreateShareRequest{Token: sharingToken(1), PreserveMarks: shoppingPtr(false)}
	first, err := f.service.CreateShare(ctx, f.author, list.ID, key, req)
	if err != nil {
		t.Fatal(err)
	}
	// Создание списка, снимок и импорт используют независимые операции при одном ключе.
	if _, err := f.service.CreateList(ctx, f.author, key, shopping.CreateListRequest{Name: "Другой"}); err != nil {
		t.Fatal(err)
	}
	f.importList(t, f.author, 1, key)
	migrationExec(t, f.pool, `
		UPDATE shopping_idempotency SET expires_at = clock_timestamp() - interval '1 second'
		WHERE owner_id = $1 AND operation = $2 AND key = $3`, f.author, fmt.Sprintf("shares:%d", list.ID), key)
	req.Token = sharingToken(2)
	second, err := f.service.CreateShare(ctx, f.author, list.ID, key, req)
	if err != nil || first.ID == second.ID {
		t.Fatal("Истёкший ключ не позволил создать новый снимок", err)
	}
	copy := f.importList(t, f.recipient, 1, key)
	migrationExec(t, f.pool, `
		UPDATE shopping_idempotency SET expires_at = clock_timestamp() - interval '1 second'
		WHERE owner_id = $1 AND operation = 'share-imports' AND key = $2`, f.recipient, key)
	if next := f.importList(t, f.recipient, 2, key); next.ID == copy.ID {
		t.Fatal("Истёкший ключ импорта не заменён новым результатом")
	}
	// Scope создания снимка включает источник, а не только пользователя и ключ.
	other, err := f.service.CreateList(ctx, f.author, "sharing-another-list-0001", shopping.CreateListRequest{Name: "Ещё"})
	if err != nil {
		t.Fatal(err)
	}
	req.Token = sharingToken(3)
	if _, err := f.service.CreateShare(ctx, f.author, other.ID, key, req); err != nil {
		t.Fatal("Ключ другого исходного списка конфликтует", err)
	}
}

type sharingResult[T any] struct {
	value T
	err   error
}

func concurrentSharing[T any](count int, action func(int) (T, error)) []sharingResult[T] {
	start := make(chan struct{})
	results := make(chan sharingResult[T], count)
	var workers sync.WaitGroup
	for i := range count {
		workers.Go(func() {
			<-start
			value, err := action(i)
			results <- sharingResult[T]{value: value, err: err}
		})
	}
	close(start)
	workers.Wait()
	close(results)
	var all []sharingResult[T]
	for result := range results {
		all = append(all, result)
	}
	return all
}

func TestSharingConcurrentActionsAndTokenCollision(t *testing.T) {
	t.Parallel()
	f := newSharingFixture(t)
	list := f.list(t)
	original := f.items(t, list)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	const count = 8
	shares := concurrentSharing(count, func(int) (shopping.Share, error) {
		return f.service.CreateShare(ctx, f.author, list.ID, "sharing-parallel-share-0001",
			shopping.CreateShareRequest{Token: sharingToken(1), PreserveMarks: shoppingPtr(true)})
	})
	for _, result := range shares {
		if result.err != nil || !reflect.DeepEqual(result.value, shares[0].value) {
			t.Fatal("Конкурентный повтор создал другой снимок", result.err)
		}
	}
	if migrationScalar(t, f.pool, `SELECT count(*)::text FROM shopping_list_shares`) != "1" {
		t.Fatal("Конкурентные повторы создали лишние снимки")
	}
	copies := concurrentSharing(count, func(int) (shopping.List, error) {
		return f.service.ImportShare(ctx, f.recipient, "sharing-parallel-import-0001",
			shopping.ImportShareRequest{Token: sharingToken(1)})
	})
	for _, result := range copies {
		if result.err != nil || !reflect.DeepEqual(result.value, copies[0].value) {
			t.Fatal("Конкурентный повтор создал другую копию", result.err)
		}
	}
	assertSharingItems(t, f, f.recipient, copies[0].value, original, true)
	for _, result := range concurrentSharing(count, func(i int) (shopping.List, error) {
		return f.service.ImportShare(ctx, f.recipient, fmt.Sprintf("sharing-parallel-new-%04d", i),
			shopping.ImportShareRequest{Token: sharingToken(1)})
	}) {
		if result.err != nil {
			t.Fatal(result.err)
		}
		assertSharingItems(t, f, f.recipient, result.value, original, true)
	}
	if got := migrationScalar(t, f.pool, `SELECT count(*)::text FROM shopping_lists WHERE owner_id = $1`, f.recipient); got != "9" {
		t.Fatal("Одинаковые действия создали дубль или разные ключи не создали новые копии")
	}
	var succeeded, conflicted int
	for _, result := range concurrentSharing(2, func(i int) (shopping.Share, error) {
		return f.service.CreateShare(ctx, f.author, list.ID, fmt.Sprintf("sharing-collision-%04d", i),
			shopping.CreateShareRequest{Token: sharingToken(2), PreserveMarks: shoppingPtr(false)})
	}) {
		if result.err == nil {
			succeeded++
		} else if errors.Is(result.err, shopping.ErrShareTokenConflict) {
			conflicted++
		} else {
			t.Fatal("Коллизия секрета потеряла доменную ошибку", result.err)
		}
	}
	if succeeded != 1 || conflicted != 1 ||
		migrationScalar(t, f.pool, `SELECT count(*)::text FROM shopping_idempotency WHERE key LIKE 'sharing-collision-%'`) != "1" {
		t.Fatal("Конфликт секрета изменил снимок или оставил запись неуспешного действия")
	}
}

type sharingGateStore struct {
	shopping.Store
	entered chan struct{}
	release chan struct{}
}

func (s sharingGateStore) InTx(ctx context.Context, action func(shopping.Transaction) error) error {
	return s.Store.InTx(ctx, func(tx shopping.Transaction) error {
		return action(sharingGateTransaction{Transaction: tx, entered: s.entered, release: s.release})
	})
}

type sharingGateTransaction struct {
	shopping.Transaction
	entered chan struct{}
	release chan struct{}
}

func (tx sharingGateTransaction) CopyListToShare(ctx context.Context, user, list, share int64) error {
	close(tx.entered)
	select {
	case <-tx.release:
		return tx.Transaction.CopyListToShare(ctx, user, list, share)
	case <-ctx.Done():
		return ctx.Err()
	}
}

func TestSharingSourceLockCoversHeaderAndItems(t *testing.T) {
	t.Parallel()
	f := newSharingFixture(t)
	list := f.list(t)
	original := f.items(t, list)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	gate := sharingGateStore{
		Store: shopping.NewRepository(f.pool), entered: make(chan struct{}), release: make(chan struct{}),
	}
	var release sync.Once
	defer release.Do(func() { close(gate.release) })
	result := make(chan sharingResult[shopping.Share], 1)
	go func() {
		share, err := shopping.NewService(gate).CreateShare(ctx, f.author, list.ID, "sharing-locked-source-0001",
			shopping.CreateShareRequest{Token: sharingToken(1), PreserveMarks: shoppingPtr(true)})
		result <- sharingResult[shopping.Share]{value: share, err: err}
	}()
	select {
	case <-gate.entered:
	case <-ctx.Done():
		t.Fatal("Создание снимка не дошло до копирования позиций")
	}
	writer, err := f.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Rollback(context.Background())
	if _, err := writer.Exec(ctx, `SET LOCAL lock_timeout = '150ms'`); err != nil {
		t.Fatal(err)
	}
	_, err = writer.Exec(ctx, `UPDATE shopping_lists SET name = 'Позднее имя' WHERE owner_id = $1 AND id = $2`, f.author, list.ID)
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "55P03" {
		t.Fatal("Правка оригинала не блокируется между чтением заголовка и позиций", err)
	}
	if err := writer.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	release.Do(func() { close(gate.release) })
	select {
	case got := <-result:
		if got.err != nil {
			t.Fatal(got.err)
		}
	case <-ctx.Done():
		t.Fatal("Создание снимка не завершилось")
	}
	if err := f.service.DeleteList(ctx, f.author, list.ID); err != nil {
		t.Fatal(err)
	}
	copy := f.importList(t, f.recipient, 1, "sharing-locked-import-0001")
	if copy.Name != list.Name {
		t.Fatal("Заголовок снимка взят из другого состояния оригинала")
	}
	assertSharingItems(t, f, f.recipient, copy, original, true)
}

func TestSharingWaitsForWriterAndRollsBackCanceledAttempt(t *testing.T) {
	t.Parallel()
	f := newSharingFixture(t)
	list := f.list(t)
	items := f.items(t, list)
	ctx := context.Background()
	writer, err := f.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Rollback(ctx)
	if _, err := writer.Exec(ctx, `SELECT id FROM shopping_lists WHERE owner_id = $1 AND id = $2 FOR UPDATE`, f.author, list.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Exec(ctx, `UPDATE shopping_lists SET name = 'После правки' WHERE id = $1`, list.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Exec(ctx, `UPDATE shopping_items SET name = 'После правки позиции' WHERE id = $1`, items[0].ID); err != nil {
		t.Fatal(err)
	}
	blocked, cancel := context.WithTimeout(ctx, 150*time.Millisecond)
	defer cancel()
	req := shopping.CreateShareRequest{Token: sharingToken(1), PreserveMarks: shoppingPtr(true)}
	_, err = f.service.CreateShare(blocked, f.author, list.ID, "sharing-waiting-writer-0001", req)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("Создание снимка не ждёт запись или теряет отмену контекста", err)
	}
	if err := writer.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if migrationScalar(t, f.pool, `SELECT count(*)::text FROM shopping_idempotency WHERE operation LIKE 'shares:%'`) != "0" {
		t.Fatal("Отменённый снимок оставил незавершённый ключ")
	}
	if _, err := f.service.CreateShare(ctx, f.author, list.ID, "sharing-waiting-writer-0001", req); err != nil {
		t.Fatal("Повтор после отмены не завершился", err)
	}
	copy := f.importList(t, f.recipient, 1, "sharing-after-writer-0001")
	items[0].Name = "После правки позиции"
	if copy.Name != "После правки" {
		t.Fatal("Снимок не увидел зафиксированное имя оригинала")
	}
	assertSharingItems(t, f, f.recipient, copy, items, true)
}

const sharingDataState = `SELECT jsonb_build_object(
	'lists', (SELECT jsonb_agg(to_jsonb(l) ORDER BY id) FROM shopping_lists l),
	'items', (SELECT jsonb_agg(to_jsonb(i) ORDER BY id) FROM shopping_items i),
	'shares', (SELECT jsonb_agg(to_jsonb(s) ORDER BY id) FROM shopping_list_shares s),
	'share_items', (SELECT jsonb_agg(to_jsonb(i) ORDER BY share_id, position) FROM shopping_list_share_items i),
	'replays', (SELECT jsonb_agg(to_jsonb(r) ORDER BY owner_id, operation, key) FROM shopping_idempotency r)
)::text`

func TestSharingFailuresRollBackResourceItemsAndReplay(t *testing.T) {
	for _, tc := range []struct {
		name       string
		importMode bool
		table      string
		event      string
		condition  string
	}{
		{"share header", false, "shopping_list_shares", "INSERT", ""},
		{"share second item", false, "shopping_list_share_items", "INSERT", "WHEN (NEW.position = 2)"},
		{"share replay", false, "shopping_idempotency", "UPDATE", "WHEN (NEW.response IS NOT NULL)"},
		{"copy header", true, "shopping_lists", "INSERT", ""},
		{"copy second item", true, "shopping_items", "INSERT", "WHEN (NEW.name = 'Второй')"},
		{"copy replay", true, "shopping_idempotency", "UPDATE", "WHEN (NEW.response IS NOT NULL)"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newSharingFixture(t)
			list := f.list(t)
			original := f.items(t, list)
			if tc.importMode {
				f.share(t, list, 1, true)
			}
			before := migrationScalar(t, f.pool, sharingDataState)
			migrationExec(t, f.pool, `CREATE FUNCTION fail_share_write() RETURNS trigger LANGUAGE plpgsql AS $$
				BEGIN RAISE EXCEPTION 'injected dependency secret'; END $$`)
			// Идентификаторы и условие берутся только из фиксированной таблицы теста.
			migrationExec(t, f.pool, "CREATE TRIGGER fail_share_write BEFORE "+tc.event+" ON "+tc.table+
				" FOR EACH ROW "+tc.condition+" EXECUTE FUNCTION fail_share_write()")
			run := func() error {
				if tc.importMode {
					_, err := f.service.ImportShare(context.Background(), f.recipient, "sharing-rollback-action-0001",
						shopping.ImportShareRequest{Token: sharingToken(1)})
					return err
				}
				_, err := f.service.CreateShare(context.Background(), f.author, list.ID, "sharing-rollback-action-0001",
					shopping.CreateShareRequest{Token: sharingToken(1), PreserveMarks: shoppingPtr(true)})
				return err
			}
			if err := run(); err == nil || strings.Contains(err.Error(), "secret") {
				t.Fatal("Ошибка записи подавлена или раскрывает детали зависимости", err)
			}
			if migrationScalar(t, f.pool, sharingDataState) != before {
				t.Fatal("Сбой оставил часть ресурса, позиций, updatedAt или записи повтора")
			}
			migrationExec(t, f.pool, "DROP TRIGGER fail_share_write ON "+tc.table)
			if err := run(); err != nil {
				t.Fatal("Повтор того же действия после rollback не завершился", err)
			}
			copy := f.importList(t, f.recipient, 1, "sharing-rollback-action-0001")
			assertSharingItems(t, f, f.recipient, copy, original, true)
		})
	}
}
