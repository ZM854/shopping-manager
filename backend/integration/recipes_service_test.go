//go:build integration

package integration_test

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ZM854/shopping-manager/backend/internal/recipe"
)

func TestRecipeServiceCRUD(t *testing.T) {
	f := newRecipeRepositoryFixture(t)
	s := recipe.NewService(f.repo)
	ctx := context.Background()
	items, err := s.GetRecipes(ctx, f.first)
	if err != nil || items == nil || len(items) != 0 {
		t.Fatalf("Пустая коллекция: %v, %v", items, err)
	}
	req := recipeRepositoryRequest("Пирог")
	first, err := s.CreateRecipe(ctx, f.first, "service-create-0001", req)
	if err != nil {
		t.Fatal(err)
	}
	checkRecipeContents(t, first, req)
	second, err := s.CreateRecipe(ctx, f.first, "service-create-0002", req)
	if err != nil || second.ID == first.ID {
		t.Fatal("Одинаковые названия должны создавать независимые рецепты", err)
	}
	items, err = s.GetRecipes(ctx, f.first)
	if err != nil || !reflect.DeepEqual(items, []recipe.Summary{first.Summary, second.Summary}) {
		t.Fatalf("Коллекция: %+v, %v", items, err)
	}
	for _, target := range []struct{ owner, id int64 }{
		{f.second, first.ID}, {f.first, second.ID + 1000},
	} {
		if _, err := s.GetRecipe(ctx, target.owner, target.id); !errors.Is(err, recipe.ErrNotFound) {
			t.Fatalf("Доступ к GET: %v", err)
		}
		if got, err := s.UpdateRecipe(ctx, target.owner, target.id, req); !errors.Is(err, recipe.ErrNotFound) || got.ID != 0 {
			t.Fatalf("Доступ к PUT: %+v, %v", got, err)
		}
		if err := s.DeleteRecipe(ctx, target.owner, target.id); !errors.Is(err, recipe.ErrNotFound) {
			t.Fatalf("Доступ к DELETE: %v", err)
		}
	}
	replacement := recipeRepositoryRequest("  После сохранения  ")
	replacement.Ingredients = []recipe.IngredientRequest{{Name: " Соль ", Unit: " по вкусу "}}
	replacement.Steps = []recipe.StepRequest{{Text: "  Вскипятить\r\n\tПосолить  "}}
	replacement.CookingTimeMinutes, replacement.CategoryCode = 5, "soups"
	updated, err := s.UpdateRecipe(ctx, f.first, first.ID, replacement)
	if err != nil {
		t.Fatal(err)
	}
	replacement.Title = "После сохранения"
	replacement.Ingredients[0] = recipe.IngredientRequest{Name: "Соль", Unit: "по вкусу"}
	replacement.Steps[0].Text = "Вскипятить\n\tПосолить"
	checkRecipeContents(t, updated, replacement)
	if updated.ID != first.ID || !updated.CreatedAt.Equal(first.CreatedAt) || !updated.UpdatedAt.After(first.UpdatedAt) {
		t.Fatal("Неверное время полной замены")
	}
	loaded, err := s.GetRecipe(ctx, f.first, first.ID)
	if err != nil || !reflect.DeepEqual(loaded, updated) {
		t.Fatal("PUT не сохранил целый рецепт", err)
	}
	if err := s.DeleteRecipe(ctx, f.first, first.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteRecipe(ctx, f.first, first.ID); !errors.Is(err, recipe.ErrNotFound) {
		t.Fatal("Повторный DELETE должен вернуть not_found", err)
	}
	loaded, err = s.GetRecipe(ctx, f.first, second.ID)
	if err != nil || !reflect.DeepEqual(loaded, second) {
		t.Fatal("Соседний рецепт изменён", err)
	}
}

func TestRecipeServiceIdempotency(t *testing.T) {
	f := newRecipeRepositoryFixture(t)
	s := recipe.NewService(f.repo)
	ctx := context.Background()
	key := "service-replay-0001"
	req := recipeRepositoryRequest("  Пирог  ")
	quantity := "2.500"
	req.Ingredients[0] = recipe.IngredientRequest{Name: " Мука ", Quantity: &quantity, Unit: " г "}
	req.Steps[0].Text = " Смешать\r\n\tИспечь "
	original, err := s.CreateRecipe(ctx, f.first, key, req)
	if err != nil {
		t.Fatal(err)
	}
	equivalent := recipeRepositoryRequest("Пирог")
	canonical := "2.5"
	equivalent.Ingredients[0] = recipe.IngredientRequest{Name: "Мука", Quantity: &canonical, Unit: "г"}
	equivalent.Steps[0].Text = "Смешать\n\tИспечь"
	for _, change := range []bool{false, true} {
		if change {
			if _, err := s.UpdateRecipe(ctx, f.first, original.ID, recipeRepositoryRequest("Изменён")); err != nil {
				t.Fatal(err)
			}
		}
		replayed, err := s.CreateRecipe(ctx, f.first, key, equivalent)
		if err != nil || !reflect.DeepEqual(replayed, original) {
			t.Fatalf("Повтор нормализованного тела должен вернуть исходный DTO: %+v, %v", replayed, err)
		}
	}
	for name, modify := range map[string]func(*recipe.SaveRequest){
		"title":    func(req *recipe.SaveRequest) { req.Title = "Другой" },
		"time":     func(req *recipe.SaveRequest) { req.CookingTimeMinutes += 5 },
		"category": func(req *recipe.SaveRequest) { req.CategoryCode = "baking" },
		"ingredients order": func(req *recipe.SaveRequest) {
			req.Ingredients[0], req.Ingredients[1] = req.Ingredients[1], req.Ingredients[0]
		},
		"steps order": func(req *recipe.SaveRequest) { req.Steps[0], req.Steps[1] = req.Steps[1], req.Steps[0] },
	} {
		t.Run(name, func(t *testing.T) {
			changed := recipeRepositoryRequest(req.Title)
			changed.Ingredients = append([]recipe.IngredientRequest(nil), req.Ingredients...)
			changed.Steps = append([]recipe.StepRequest(nil), req.Steps...)
			modify(&changed)
			if got, err := s.CreateRecipe(ctx, f.first, key, changed); !errors.Is(err, recipe.ErrIdempotencyConflict) || got.ID != 0 {
				t.Fatalf("Другой payload принят: %+v, %v", got, err)
			}
		})
	}
	other, err := s.CreateRecipe(ctx, f.second, key, equivalent)
	if err != nil || other.ID == original.ID {
		t.Fatal("Ключи разных владельцев должны быть независимы", err)
	}
	if err := s.DeleteRecipe(ctx, f.first, original.ID); err != nil {
		t.Fatal(err)
	}
	if got, err := s.CreateRecipe(ctx, f.first, key, equivalent); !errors.Is(err, recipe.ErrResourceDeleted) || got.ID != 0 {
		t.Fatalf("Удалённый результат: %+v, %v", got, err)
	}
	newRecipe, err := s.CreateRecipe(ctx, f.first, "service-replay-0002", equivalent)
	if err != nil || newRecipe.ID == original.ID {
		t.Fatal("Новое действие после удаления должно создать рецепт", err)
	}
	migrationExec(t, f.pool, `
		UPDATE recipe_idempotency SET expires_at = statement_timestamp() - interval '1 second'
		WHERE owner_id = $1 AND key = $2`, f.first, key)
	afterExpiry, err := s.CreateRecipe(ctx, f.first, key, recipeRepositoryRequest("После истечения"))
	if err != nil || afterExpiry.ID == original.ID {
		t.Fatal("Истёкший ключ не переиспользован", err)
	}
}

const recipeServiceState = `
	SELECT jsonb_build_array(
		(SELECT COALESCE(jsonb_agg(to_jsonb(r) ORDER BY id), '[]') FROM recipes r),
		(SELECT COALESCE(jsonb_agg(to_jsonb(i) ORDER BY recipe_id, position), '[]') FROM recipe_ingredients i),
		(SELECT COALESCE(jsonb_agg(to_jsonb(s) ORDER BY recipe_id, position), '[]') FROM recipe_steps s),
		(SELECT COALESCE(jsonb_agg(to_jsonb(k) ORDER BY owner_id, key), '[]') FROM recipe_idempotency k)
	)::text`

func TestRecipeServiceRollback(t *testing.T) {
	for _, tc := range []struct {
		name, operation, table, event string
		deferred                      bool
	}{
		{"ingredient", "create", "recipe_ingredients", "INSERT", false},
		{"step", "create", "recipe_steps", "INSERT", false},
		{"replay", "create", "recipe_idempotency", "UPDATE", false},
		{"commit", "create", "recipe_idempotency", "UPDATE", true},
		{"replace", "update", "recipe_steps", "INSERT", false},
		{"replace commit", "update", "recipes", "UPDATE", true},
		{"delete cascade", "delete", "recipe_steps", "DELETE", false},
		{"delete commit", "delete", "recipes", "DELETE", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newRecipeRepositoryFixture(t)
			s := recipe.NewService(f.repo)
			ctx := context.Background()
			original, err := s.CreateRecipe(ctx, f.first, "service-original-0001", recipeRepositoryRequest("Оригинал"))
			if err != nil {
				t.Fatal(err)
			}
			before := migrationScalar(t, f.pool, recipeServiceState)
			migrationExec(t, f.pool, `CREATE FUNCTION fail_recipe_write() RETURNS trigger LANGUAGE plpgsql AS $$
				BEGIN RAISE EXCEPTION 'injected private recipe text'; END $$`)
			trigger := "CREATE TRIGGER fail_recipe_write BEFORE "
			options := ""
			if tc.deferred {
				trigger = "CREATE CONSTRAINT TRIGGER fail_recipe_write AFTER "
				options = " DEFERRABLE INITIALLY DEFERRED"
			}
			// Идентификаторы берутся из фиксированной таблицы теста, не из запроса.
			migrationExec(t, f.pool, trigger+tc.event+" ON "+tc.table+options+
				" FOR EACH ROW EXECUTE FUNCTION fail_recipe_write()")
			run := func() (recipe.Recipe, error) {
				switch tc.operation {
				case "create":
					return s.CreateRecipe(ctx, f.first, "service-rollback-0001", recipeRepositoryRequest("Новое"))
				case "update":
					return s.UpdateRecipe(ctx, f.first, original.ID, recipeRepositoryRequest("Правка"))
				default:
					return recipe.Recipe{}, s.DeleteRecipe(ctx, f.first, original.ID)
				}
			}
			got, err := run()
			if err == nil || strings.Contains(err.Error(), "private") || !reflect.DeepEqual(got, recipe.Recipe{}) {
				t.Fatalf("Ошибка выдана за успех или раскрывает данные: %+v, %v", got, err)
			}
			if migrationScalar(t, f.pool, recipeServiceState) != before {
				t.Fatal("Сбой не откатил заголовок/содержимое/время/replay")
			}
			migrationExec(t, f.pool, "DROP TRIGGER fail_recipe_write ON "+tc.table)
			if _, err := run(); err != nil {
				t.Fatal("Повтор после rollback не завершился", err)
			}
		})
	}
}

type recipeServiceResult struct {
	value recipe.Recipe
	err   error
}

func TestRecipeServiceConcurrentCreates(t *testing.T) {
	for _, sameBody := range []bool{true, false} {
		t.Run(fmt.Sprintf("sameBody=%t", sameBody), func(t *testing.T) {
			f := newRecipeRepositoryFixture(t)
			s := recipe.NewService(f.repo)
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			const workers = 6
			start := make(chan struct{})
			results := make(chan recipeServiceResult, workers)
			var ready sync.WaitGroup
			ready.Add(workers)
			for i := range workers {
				go func() {
					req := recipeRepositoryRequest("Один рецепт")
					if !sameBody {
						req.Title = fmt.Sprintf("Вариант %d", i)
					}
					ready.Done()
					<-start
					got, err := s.CreateRecipe(ctx, f.first, "service-concurrent-0001", req)
					results <- recipeServiceResult{got, err}
				}()
			}
			ready.Wait()
			close(start)
			success, conflicts := 0, 0
			var original recipe.Recipe
			for range workers {
				got := <-results
				if got.err == nil {
					if success > 0 && !reflect.DeepEqual(got.value, original) {
						t.Fatal("Параллельные повторы вернули разные DTO")
					}
					original = got.value
					success++
				} else if errors.Is(got.err, recipe.ErrIdempotencyConflict) && got.value.ID == 0 {
					conflicts++
				} else {
					t.Fatal(got.err)
				}
			}
			if (sameBody && (success != workers || conflicts != 0)) ||
				(!sameBody && (success != 1 || conflicts != workers-1)) ||
				migrationScalar(t, f.pool, "SELECT count(*)::text FROM recipes") != "1" ||
				migrationScalar(t, f.pool, "SELECT count(*)::text FROM recipe_idempotency") != "1" {
				t.Fatalf("Параллельное создание: success=%d, conflict=%d", success, conflicts)
			}
		})
	}
}

type recipeGateStore struct {
	recipe.Store
	at      string
	entered chan struct{}
	release chan struct{}
	failure error
}

func (s recipeGateStore) InTx(ctx context.Context, action func(recipe.Transaction) error) error {
	return s.Store.InTx(ctx, func(tx recipe.Transaction) error {
		return action(recipeGateTransaction{Transaction: tx, gate: s})
	})
}

type recipeGateTransaction struct {
	recipe.Transaction
	gate recipeGateStore
}

func (tx recipeGateTransaction) wait(ctx context.Context, at string) error {
	if tx.gate.at != at {
		return nil
	}
	close(tx.gate.entered)
	select {
	case <-tx.gate.release:
		return tx.gate.failure
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (tx recipeGateTransaction) LockRecipe(ctx context.Context, owner, id int64) error {
	if err := tx.Transaction.LockRecipe(ctx, owner, id); err != nil {
		return err
	}
	return tx.wait(ctx, "lock")
}

func (tx recipeGateTransaction) SaveReplay(ctx context.Context, owner int64, key string, id int64, body []byte) error {
	if err := tx.Transaction.SaveReplay(ctx, owner, key, id, body); err != nil {
		return err
	}
	return tx.wait(ctx, "replay")
}

func awaitRecipeResult(t *testing.T, ctx context.Context, result <-chan recipeServiceResult) recipeServiceResult {
	t.Helper()
	select {
	case got := <-result:
		return got
	case <-ctx.Done():
		t.Fatal("Операция не завершилась до таймаута")
		return recipeServiceResult{}
	}
}

func TestRecipeServiceConcurrentWrites(t *testing.T) {
	for _, mode := range []string{"update/update", "update/delete", "delete/update"} {
		t.Run(mode, func(t *testing.T) {
			f := newRecipeRepositoryFixture(t)
			s := recipe.NewService(f.repo)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			original, err := s.CreateRecipe(ctx, f.first, "service-before-write-0001", recipeRepositoryRequest("До правок"))
			if err != nil {
				t.Fatal(err)
			}
			gate := recipeGateStore{Store: f.repo, at: "lock", entered: make(chan struct{}), release: make(chan struct{})}
			var release sync.Once
			defer release.Do(func() { close(gate.release) })
			first := make(chan recipeServiceResult, 1)
			go func() {
				firstService := recipe.NewService(gate)
				if mode == "delete/update" {
					first <- recipeServiceResult{err: firstService.DeleteRecipe(ctx, f.first, original.ID)}
					return
				}
				got, err := firstService.UpdateRecipe(ctx, f.first, original.ID, recipeRepositoryRequest("Первая правка"))
				first <- recipeServiceResult{got, err}
			}()
			select {
			case <-gate.entered:
			case <-ctx.Done():
				t.Fatal("Service не заблокировал рецепт перед записью")
			}
			second := make(chan recipeServiceResult, 1)
			go func() {
				if mode == "update/delete" {
					second <- recipeServiceResult{err: s.DeleteRecipe(ctx, f.first, original.ID)}
					return
				}
				got, err := s.UpdateRecipe(ctx, f.first, original.ID, recipeRepositoryRequest("Последняя правка"))
				second <- recipeServiceResult{got, err}
			}()
			loaded, err := s.GetRecipe(ctx, f.first, original.ID)
			if err != nil || !reflect.DeepEqual(loaded, original) {
				t.Fatal("Читатель не получил исходный рецепт при занятой строке", err)
			}
			blocked, cancelBlocked := context.WithTimeout(ctx, 200*time.Millisecond)
			err = s.DeleteRecipe(blocked, f.first, original.ID)
			cancelBlocked()
			if !errors.Is(err, context.DeadlineExceeded) {
				t.Fatal("DELETE не ждёт занятую строку", err)
			}
			select {
			case got := <-second:
				t.Fatalf("Конкурирующая запись обошла блокировку: %+v", got)
			default:
			}
			release.Do(func() { close(gate.release) })
			if got := awaitRecipeResult(t, ctx, first); got.err != nil {
				t.Fatal(got.err)
			}
			got := awaitRecipeResult(t, ctx, second)
			if mode == "delete/update" {
				if !errors.Is(got.err, recipe.ErrNotFound) || got.value.ID != 0 {
					t.Fatal("PUT после удаления восстановил рецепт", got.err)
				}
			} else if got.err != nil {
				t.Fatal(got.err)
			}
			loaded, err = s.GetRecipe(ctx, f.first, original.ID)
			if mode == "update/update" {
				if err != nil || !reflect.DeepEqual(loaded, got.value) {
					t.Fatal("Последнее сохранение не победило", err)
				}
				checkRecipeContents(t, loaded, recipeRepositoryRequest("Последняя правка"))
			} else if !errors.Is(err, recipe.ErrNotFound) {
				t.Fatal("Удалённый рецепт сохранился", err)
			}
		})
	}
}

func TestRecipeServiceConcurrentRetryAfterRollback(t *testing.T) {
	f := newRecipeRepositoryFixture(t)
	s := recipe.NewService(f.repo)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	failure := errors.New("injected private failure")
	gate := recipeGateStore{
		Store: f.repo, at: "replay", entered: make(chan struct{}), release: make(chan struct{}), failure: failure,
	}
	var release sync.Once
	defer release.Do(func() { close(gate.release) })
	key, req := "service-waiting-retry-0001", recipeRepositoryRequest("Повтор")
	first := make(chan recipeServiceResult, 1)
	go func() {
		got, err := recipe.NewService(gate).CreateRecipe(ctx, f.first, key, req)
		first <- recipeServiceResult{got, err}
	}()
	select {
	case <-gate.entered:
	case <-ctx.Done():
		t.Fatal("Создание не дошло до сохранения replay")
	}
	// Уже записанные заголовок/массивы/replay ещё не видны за пределами транзакции.
	items, err := s.GetRecipes(ctx, f.first)
	if err != nil || len(items) != 0 {
		t.Fatal("Незакоммиченное создание видно читателю", err)
	}
	second := make(chan recipeServiceResult, 1)
	go func() {
		got, err := s.CreateRecipe(ctx, f.first, key, req)
		second <- recipeServiceResult{got, err}
	}()
	blocked, cancelBlocked := context.WithTimeout(ctx, 200*time.Millisecond)
	_, err = s.CreateRecipe(blocked, f.first, key, req)
	cancelBlocked()
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("POST не ждёт незакоммиченный claim", err)
	}
	release.Do(func() { close(gate.release) })
	failed := awaitRecipeResult(t, ctx, first)
	if !errors.Is(failed.err, failure) || failed.value.ID != 0 {
		t.Fatal("Первое создание не откатилось", failed.err)
	}
	succeeded := awaitRecipeResult(t, ctx, second)
	if succeeded.err != nil {
		t.Fatal("Ожидающий повтор не создал рецепт после rollback", succeeded.err)
	}
	checkRecipeContents(t, succeeded.value, req)
	replayed, err := s.CreateRecipe(ctx, f.first, key, req)
	if err != nil || !reflect.DeepEqual(replayed, succeeded.value) ||
		migrationScalar(t, f.pool, "SELECT count(*)::text FROM recipes") != "1" ||
		migrationScalar(t, f.pool, "SELECT count(*)::text FROM recipe_idempotency") != "1" {
		t.Fatal("После rollback остался лишний рецепт или неверный replay", err)
	}
}
