//go:build integration

package integration_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ZM854/shopping-manager/backend/internal/recipe"
	"github.com/ZM854/shopping-manager/backend/internal/testutil/testdb"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type recipeRepositoryFixture struct {
	pool          *pgxpool.Pool
	repo          *recipe.Repository
	first, second int64
}

func newRecipeRepositoryFixture(t *testing.T) *recipeRepositoryFixture {
	t.Helper()
	pool := testdb.New(t)
	testdb.ApplyMigrations(t, pool, 0, 10)
	return &recipeRepositoryFixture{
		pool: pool, repo: recipe.NewRepository(pool),
		first:  migrationUser(t, pool, "recipe-first@example.test"),
		second: migrationUser(t, pool, "recipe-second@example.test"),
	}
}

func recipeRepositoryRequest(title string) recipe.SaveRequest {
	quantity := "999999999.999"
	return recipe.SaveRequest{
		Title: title, CookingTimeMinutes: 15, CategoryCode: "main-dishes",
		Ingredients: []recipe.IngredientRequest{
			{Name: "Мука", Quantity: &quantity, Unit: "г"},
			{Name: "Соль", Unit: "по вкусу"},
			{Name: "Мука", Quantity: &quantity},
		},
		Steps: []recipe.StepRequest{{Text: "Смешать\n\tтщательно"}, {Text: "Испечь"}},
	}
}

func (f *recipeRepositoryFixture) create(t *testing.T, owner int64, req recipe.SaveRequest) recipe.Recipe {
	t.Helper()
	var result recipe.Recipe
	err := f.repo.InTx(context.Background(), func(tx recipe.Transaction) error {
		var err error
		result, err = tx.CreateRecipe(context.Background(), owner, req)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func checkRecipeContents(t *testing.T, got recipe.Recipe, req recipe.SaveRequest) {
	t.Helper()
	if got.Title != req.Title || got.CookingTimeMinutes != req.CookingTimeMinutes ||
		got.CategoryCode != req.CategoryCode || len(got.Ingredients) != len(req.Ingredients) ||
		len(got.Steps) != len(req.Steps) {
		t.Fatalf("Неверный рецепт: %+v", got)
	}
	for i, ingredient := range got.Ingredients {
		want := req.Ingredients[i]
		if ingredient.Position != i+1 || ingredient.Name != want.Name ||
			ingredient.Unit != want.Unit || !reflect.DeepEqual(ingredient.Quantity, want.Quantity) {
			t.Fatalf("Ингредиент %d: %+v вместо %+v", i, ingredient, want)
		}
	}
	for i, step := range got.Steps {
		if step.Position != i+1 || step.Text != req.Steps[i].Text {
			t.Fatalf("Шаг %d: %+v", i, step)
		}
	}
	if got.ID <= 0 || got.CreatedAt.IsZero() || got.UpdatedAt.Before(got.CreatedAt) ||
		got.CreatedAt.Location() != time.UTC || got.UpdatedAt.Location() != time.UTC {
		t.Fatalf("Неверные ID/время: %+v", got.Summary)
	}
}

func TestRecipeRepositoryCRUD(t *testing.T) {
	f := newRecipeRepositoryFixture(t)
	ctx := context.Background()
	items, err := f.repo.GetRecipes(ctx, f.first)
	if err != nil || items == nil || len(items) != 0 {
		t.Fatalf("Пустая коллекция: %v, %v", items, err)
	}
	req := recipeRepositoryRequest("Омлет")
	first := f.create(t, f.first, req)
	second := f.create(t, f.first, req)
	f.create(t, f.second, recipeRepositoryRequest("Чужой"))
	checkRecipeContents(t, first, req)
	items, err = f.repo.GetRecipes(ctx, f.first)
	if err != nil || !reflect.DeepEqual(items, []recipe.Summary{first.Summary, second.Summary}) {
		t.Fatalf("Коллекция/порядок/владелец: %+v, %v", items, err)
	}
	loaded, err := f.repo.GetRecipe(ctx, f.first, first.ID)
	if err != nil || !reflect.DeepEqual(loaded, first) {
		t.Fatalf("Повторное чтение: %+v, %v", loaded, err)
	}

	replacement := recipeRepositoryRequest("Новый рецепт")
	replacement.CookingTimeMinutes, replacement.CategoryCode = 1440, "other"
	minimum := "0.001"
	replacement.Ingredients = []recipe.IngredientRequest{{Name: "Вода", Quantity: &minimum}}
	replacement.Steps = []recipe.StepRequest{{Text: strings.Repeat("я", recipe.MaxStepTextLength)}}
	var updated recipe.Recipe
	err = f.repo.InTx(ctx, func(tx recipe.Transaction) error {
		if err := tx.LockRecipe(ctx, f.first, first.ID); err != nil {
			return err
		}
		var err error
		updated, err = tx.UpdateRecipe(ctx, f.first, first.ID, replacement)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	checkRecipeContents(t, updated, replacement)
	if updated.ID != first.ID || !updated.CreatedAt.Equal(first.CreatedAt) ||
		!updated.UpdatedAt.After(first.UpdatedAt) {
		t.Fatalf("Изменение метаданных: %+v -> %+v", first.Summary, updated.Summary)
	}
	loaded, err = f.repo.GetRecipe(ctx, f.first, first.ID)
	if err != nil || !reflect.DeepEqual(loaded, updated) {
		t.Fatalf("Замена не сохранена: %+v, %v", loaded, err)
	}
	if err := f.repo.InTx(ctx, func(tx recipe.Transaction) error {
		if err := tx.LockRecipe(ctx, f.first, first.ID); err != nil {
			return err
		}
		return tx.DeleteRecipe(ctx, f.first, first.ID)
	}); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"recipe_ingredients", "recipe_steps"} {
		if count := migrationScalar(t, f.pool, "SELECT count(*)::text FROM "+table+" WHERE recipe_id=$1", first.ID); count != "0" {
			t.Fatalf("Каскад %s: %s", table, count)
		}
	}
	remaining, err := f.repo.GetRecipe(ctx, f.first, second.ID)
	if err != nil || !reflect.DeepEqual(remaining, second) {
		t.Fatalf("Изменён соседний рецепт: %+v, %v", remaining, err)
	}
}

func TestRecipeRepositoryOwnerAndMissing(t *testing.T) {
	f := newRecipeRepositoryFixture(t)
	ctx := context.Background()
	original := f.create(t, f.first, recipeRepositoryRequest("Личный"))
	for _, target := range []struct {
		name  string
		owner int64
		id    int64
	}{
		{"foreign", f.second, original.ID},
		{"missing", f.first, original.ID + 1000},
	} {
		t.Run(target.name, func(t *testing.T) {
			if _, err := f.repo.GetRecipe(ctx, target.owner, target.id); !errors.Is(err, recipe.ErrNotFound) {
				t.Fatalf("GET: %v", err)
			}
			for name, action := range map[string]func(recipe.Transaction) error{
				"lock": func(tx recipe.Transaction) error { return tx.LockRecipe(ctx, target.owner, target.id) },
				"update": func(tx recipe.Transaction) error {
					_, err := tx.UpdateRecipe(ctx, target.owner, target.id, recipeRepositoryRequest("Попытка"))
					return err
				},
				"delete": func(tx recipe.Transaction) error { return tx.DeleteRecipe(ctx, target.owner, target.id) },
			} {
				if err := f.repo.InTx(ctx, action); !errors.Is(err, recipe.ErrNotFound) {
					t.Fatalf("%s: %v", name, err)
				}
			}
		})
	}
	loaded, err := f.repo.GetRecipe(ctx, f.first, original.ID)
	if err != nil || !reflect.DeepEqual(loaded, original) {
		t.Fatalf("Чужая операция изменила рецепт: %+v, %v", loaded, err)
	}
}

func TestRecipeRepositoryReadOrderAndExactQuantity(t *testing.T) {
	f := newRecipeRepositoryFixture(t)
	ctx := context.Background()
	id := recipeMigrationCreate(t, f.pool, f.first)
	var recipeID int64
	if _, err := fmt.Sscan(id, &recipeID); err != nil {
		t.Fatal(err)
	}
	empty, err := f.repo.GetRecipe(ctx, f.first, recipeID)
	if err != nil || empty.Ingredients == nil || empty.Steps == nil ||
		len(empty.Ingredients) != 0 || len(empty.Steps) != 0 {
		t.Fatalf("Чтение пустого содержимого: %+v, %v", empty, err)
	}
	// Физический порядок вставки не должен влиять на порядок DTO.
	migrationExec(t, f.pool, `
		INSERT INTO recipe_ingredients (recipe_id, position, name, quantity, unit)
		VALUES ($1, 3, 'Повтор', NULL, 'по вкусу'),
			($1, 2, 'Повтор', 2.500, ''), ($1, 1, 'Мука', 999999999.999, 'г')`, recipeID)
	migrationExec(t, f.pool, `
		INSERT INTO recipe_steps (recipe_id, position, text)
		VALUES ($1, 2, 'Второй'), ($1, 1, 'Первый')`, recipeID)
	loaded, err := f.repo.GetRecipe(ctx, f.first, recipeID)
	if err != nil {
		t.Fatal(err)
	}
	maxQuantity, normalized := "999999999.999", "2.5"
	wantIngredients := []recipe.Ingredient{
		{Position: 1, Name: "Мука", Quantity: &maxQuantity, Unit: "г"},
		{Position: 2, Name: "Повтор", Quantity: &normalized},
		{Position: 3, Name: "Повтор", Unit: "по вкусу"},
	}
	if !reflect.DeepEqual(loaded.Ingredients, wantIngredients) ||
		!reflect.DeepEqual(loaded.Steps, []recipe.Step{{Position: 1, Text: "Первый"}, {Position: 2, Text: "Второй"}}) {
		t.Fatalf("Порядок/decimal/null: %+v, %+v", loaded.Ingredients, loaded.Steps)
	}
}

func TestRecipeRepositoryRollback(t *testing.T) {
	f := newRecipeRepositoryFixture(t)
	ctx := context.Background()
	original := f.create(t, f.first, recipeRepositoryRequest("До ошибки"))
	for _, operation := range []string{"create", "update"} {
		t.Run(operation, func(t *testing.T) {
			bad := recipeRepositoryRequest("Не сохранять")
			// Ошибка последней записи должна откатить заголовок, ингредиенты и claim.
			bad.Steps[1].Text = ""
			err := f.repo.InTx(ctx, func(tx recipe.Transaction) error {
				if _, err := tx.Claim(ctx, f.first, "rollback-key-0001", strings.Repeat("a", 64), time.Now().Add(time.Hour)); err != nil {
					return err
				}
				if operation == "create" {
					_, err := tx.CreateRecipe(ctx, f.first, bad)
					return err
				}
				if err := tx.LockRecipe(ctx, f.first, original.ID); err != nil {
					return err
				}
				_, err := tx.UpdateRecipe(ctx, f.first, original.ID, bad)
				return err
			})
			var pgErr *pgconn.PgError
			if !errors.As(err, &pgErr) || pgErr.Code != "23514" {
				t.Fatalf("Ожидался CHECK, получено: %v", err)
			}
			loaded, err := f.repo.GetRecipe(ctx, f.first, original.ID)
			if err != nil || !reflect.DeepEqual(loaded, original) {
				t.Fatalf("Не откатилось содержимое/время: %+v, %v", loaded, err)
			}
			if migrationScalar(t, f.pool, "SELECT count(*)::text FROM recipes") != "1" ||
				migrationScalar(t, f.pool, "SELECT count(*)::text FROM recipe_idempotency") != "0" {
				t.Fatal("Не откатились создание/claim")
			}
		})
	}
	rollback := errors.New("rollback callback")
	err := f.repo.InTx(ctx, func(tx recipe.Transaction) error {
		if err := tx.DeleteRecipe(ctx, f.first, original.ID); err != nil {
			return err
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatal(err)
	}
	loaded, err := f.repo.GetRecipe(ctx, f.first, original.ID)
	if err != nil || !reflect.DeepEqual(loaded, original) {
		t.Fatal("Не откатилось удаление с каскадом", err)
	}
	// Массив decimal передаётся как text[] и преобразуется в NUMERIC без typmod:
	// лишняя точность должна дойти до CHECK, а не округлиться при передаче.
	tooPrecise := "1.0001"
	badQuantity := recipeRepositoryRequest("Не округлять")
	badQuantity.Ingredients[0].Quantity = &tooPrecise
	err = f.repo.InTx(ctx, func(tx recipe.Transaction) error {
		_, err := tx.CreateRecipe(ctx, f.first, badQuantity)
		return err
	})
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23514" {
		t.Fatalf("Точность количества: %v", err)
	}
	if migrationScalar(t, f.pool, "SELECT count(*)::text FROM recipes") != "1" {
		t.Fatal("Некорректное количество оставило заголовок")
	}
}

func TestRecipeRepositoryCanceledContext(t *testing.T) {
	f := newRecipeRepositoryFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := f.repo.GetRecipes(ctx, f.first); !errors.Is(err, context.Canceled) {
		t.Fatalf("Отмена коллекции: %v", err)
	}
	if _, err := f.repo.GetRecipe(ctx, f.first, 1); !errors.Is(err, context.Canceled) {
		t.Fatalf("Отмена GET: %v", err)
	}
	if err := f.repo.InTx(ctx, func(recipe.Transaction) error {
		t.Error("Callback вызван после отмены Begin")
		return nil
	}); !errors.Is(err, context.Canceled) {
		t.Fatalf("Отмена Begin: %v", err)
	}

	commitCtx, cancelCommit := context.WithCancel(context.Background())
	defer cancelCommit()
	err := f.repo.InTx(commitCtx, func(tx recipe.Transaction) error {
		if _, err := tx.CreateRecipe(commitCtx, f.first, recipeRepositoryRequest("Отмена commit")); err != nil {
			return err
		}
		cancelCommit()
		return nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Отмена Commit: %v", err)
	}
	if migrationScalar(t, f.pool, "SELECT count(*)::text FROM recipes") != "0" {
		t.Fatal("Отменённая транзакция зафиксирована")
	}
}

func TestRecipeRepositoryReplay(t *testing.T) {
	f := newRecipeRepositoryFixture(t)
	ctx := context.Background()
	key, hash := "recipe-replay-0001", strings.Repeat("a", 64)
	var original recipe.Recipe
	err := f.repo.InTx(ctx, func(tx recipe.Transaction) error {
		claim, err := tx.Claim(ctx, f.first, key, hash, time.Now().Add(time.Hour))
		if err != nil || claim.Hash != hash || claim.ResourceID != nil || claim.Response != nil {
			return fmt.Errorf("новый claim: %+v, %v", claim, err)
		}
		original, err = tx.CreateRecipe(ctx, f.first, recipeRepositoryRequest("Исходный"))
		if err != nil {
			return err
		}
		body, err := json.Marshal(original)
		if err != nil {
			return err
		}
		return tx.SaveReplay(ctx, f.first, key, original.ID, body)
	})
	if err != nil {
		t.Fatal(err)
	}
	if migrationScalar(t, f.pool, `
		SELECT (expires_at >= statement_timestamp() + interval '23 hours 59 minutes')::text
		FROM recipe_idempotency WHERE owner_id=$1 AND key=$2`, f.first, key) != "true" {
		t.Fatal("Сохранённый replay доступен менее 24 часов")
	}
	// Изменение и удаление результата не изменяют сохранённый ответ.
	for _, operation := range []string{"update", "delete"} {
		err := f.repo.InTx(ctx, func(tx recipe.Transaction) error {
			if operation == "update" {
				_, err := tx.UpdateRecipe(ctx, f.first, original.ID, recipeRepositoryRequest("Другой"))
				return err
			}
			return tx.DeleteRecipe(ctx, f.first, original.ID)
		})
		if err != nil {
			t.Fatal(err)
		}
		err = f.repo.InTx(ctx, func(tx recipe.Transaction) error {
			replay, err := tx.Claim(ctx, f.first, key, strings.Repeat("b", 64), time.Now().Add(48*time.Hour))
			if err != nil {
				return err
			}
			var decoded recipe.Recipe
			if err := json.Unmarshal(replay.Response, &decoded); err != nil {
				return err
			}
			if replay.Hash != hash || replay.ResourceID == nil || *replay.ResourceID != original.ID ||
				!reflect.DeepEqual(decoded, original) {
				return fmt.Errorf("изменён исходный replay: %+v", replay)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	// Такой же ключ другого владельца независим; сохранить чужой claim нельзя.
	err = f.repo.InTx(ctx, func(tx recipe.Transaction) error {
		if err := tx.SaveReplay(ctx, f.second, key, original.ID, []byte(`{}`)); !errors.Is(err, recipe.ErrNotFound) {
			return fmt.Errorf("чужой SaveReplay: %v", err)
		}
		other, err := tx.Claim(ctx, f.second, key, strings.Repeat("b", 64), time.Now().Add(time.Hour))
		if err != nil || other.ResourceID != nil || other.Hash != strings.Repeat("b", 64) {
			return fmt.Errorf("область владельца: %+v, %v", other, err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	migrationExec(t, f.pool, `
		UPDATE recipe_idempotency SET expires_at = statement_timestamp() - interval '1 second'
		WHERE owner_id=$1 AND key=$2`, f.first, key)
	err = f.repo.InTx(ctx, func(tx recipe.Transaction) error {
		reclaimed, err := tx.Claim(ctx, f.first, key, strings.Repeat("c", 64), time.Now().Add(24*time.Hour))
		if err != nil || reclaimed.Hash != strings.Repeat("c", 64) ||
			reclaimed.ResourceID != nil || reclaimed.Response != nil {
			return fmt.Errorf("истёкший claim: %+v, %v", reclaimed, err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestRecipeRepositoryLocks(t *testing.T) {
	f := newRecipeRepositoryFixture(t)
	ctx := context.Background()
	original := f.create(t, f.first, recipeRepositoryRequest("Блокировка"))
	other := f.create(t, f.second, recipeRepositoryRequest("Независимый"))
	for _, kind := range []string{"recipe", "claim"} {
		t.Run(kind, func(t *testing.T) {
			lock := func(ctx context.Context, tx recipe.Transaction, owner, id int64) error {
				if kind == "recipe" {
					return tx.LockRecipe(ctx, owner, id)
				}
				_, err := tx.Claim(ctx, owner, "parallel-key-0001", strings.Repeat("a", 64), time.Now().Add(time.Hour))
				return err
			}
			err := f.repo.InTx(ctx, func(held recipe.Transaction) error {
				if err := lock(ctx, held, f.first, original.ID); err != nil {
					return err
				}
				// Независимый ресурс не должен ждать удерживаемую блокировку.
				independent, cancel := context.WithTimeout(ctx, 3*time.Second)
				defer cancel()
				if err := f.repo.InTx(independent, func(tx recipe.Transaction) error {
					return lock(independent, tx, f.second, other.ID)
				}); err != nil {
					return fmt.Errorf("независимая операция: %w", err)
				}
				blocked, cancelBlocked := context.WithTimeout(ctx, 200*time.Millisecond)
				defer cancelBlocked()
				err := f.repo.InTx(blocked, func(tx recipe.Transaction) error {
					return lock(blocked, tx, f.first, original.ID)
				})
				if !errors.Is(err, context.DeadlineExceeded) {
					return fmt.Errorf("вторая операция не ждала блокировку: %v", err)
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
			// После commit блокировка отпущена, а пул работоспособен после отмены.
			if err := f.repo.InTx(ctx, func(tx recipe.Transaction) error {
				return lock(ctx, tx, f.first, original.ID)
			}); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestRecipeRepositoryConsistentRead(t *testing.T) {
	f := newRecipeRepositoryFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	version := func(marker string) recipe.SaveRequest {
		req := recipeRepositoryRequest(marker)
		req.Ingredients = []recipe.IngredientRequest{{Name: marker}}
		req.Steps = []recipe.StepRequest{{Text: marker}}
		return req
	}
	original := f.create(t, f.first, version("A"))
	err := f.repo.InTx(ctx, func(tx recipe.Transaction) error {
		if _, err := tx.UpdateRecipe(ctx, f.first, original.ID, version("B")); err != nil {
			return err
		}
		// Незакоммиченная замена не видна читателю и не блокирует его.
		loaded, err := f.repo.GetRecipe(ctx, f.first, original.ID)
		if err != nil || !reflect.DeepEqual(loaded, original) {
			return fmt.Errorf("чтение до commit: %+v, %v", loaded, err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := f.repo.GetRecipe(ctx, f.first, original.ID)
	if err != nil {
		t.Fatal(err)
	}
	checkRecipeContents(t, loaded, version("B"))

	writer := make(chan error, 1)
	go func() {
		for i := range 80 {
			marker := fmt.Sprintf("Версия %d", i)
			if err := f.repo.InTx(ctx, func(tx recipe.Transaction) error {
				_, err := tx.UpdateRecipe(ctx, f.first, original.ID, version(marker))
				return err
			}); err != nil {
				writer <- err
				return
			}
		}
		writer <- nil
	}()
	// Даже когда commit попадает между чтениями частей, DTO должен быть цельным.
	var readErr error
	for range 160 {
		got, err := f.repo.GetRecipe(ctx, f.first, original.ID)
		if err != nil {
			readErr = err
			break
		}
		if len(got.Ingredients) != 1 || len(got.Steps) != 1 ||
			got.Title != got.Ingredients[0].Name || got.Title != got.Steps[0].Text {
			readErr = fmt.Errorf("смешаны версии: %+v", got)
			break
		}
	}
	if err := <-writer; err != nil {
		t.Fatal(err)
	}
	if readErr != nil {
		t.Fatal(readErr)
	}
}
