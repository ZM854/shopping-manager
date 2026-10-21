//go:build integration

package integration_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ZM854/shopping-manager/backend/internal/auth"
	"github.com/ZM854/shopping-manager/backend/internal/config"
	"github.com/ZM854/shopping-manager/backend/internal/middleware"
	"github.com/ZM854/shopping-manager/backend/internal/router"
	"github.com/ZM854/shopping-manager/backend/internal/shopping"
	"github.com/gin-gonic/gin"
)

type sharingHTTPFixture struct {
	*sharingFixture
	router *gin.Engine
	tokens map[int64]string
	logs   bytes.Buffer
}

func newSharingHTTPFixture(t *testing.T) *sharingHTTPFixture {
	t.Helper()
	f := &sharingHTTPFixture{sharingFixture: newSharingFixture(t), tokens: make(map[int64]string)}
	log := slog.New(slog.NewJSONHandler(&f.logs, nil))
	tokens := auth.NewTokenService(log, nil,
		"sharing-test-access-key-at-least-32-bytes", "sharing-test-refresh-key-at-least-32-bytes", time.Hour, time.Hour)
	for _, user := range []int64{f.author, f.recipient} {
		pair, err := tokens.GenerateTokens(user)
		if err != nil {
			t.Fatal(err)
		}
		f.tokens[user] = pair.AccessToken
	}
	f.router = router.New(log, shopping.NewHandler(f.service, log), auth.NewAuthHandler(log, nil, false),
		middleware.NewAuthMiddleware(tokens), config.Config{AllowedOrigins: []string{"http://client.example.test"}})
	return f
}

func (f *sharingHTTPFixture) request(user int64, method, path, body, key string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, "/api"+path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://client.example.test")
	if user != 0 {
		req.Header.Set("Authorization", "Bearer "+f.tokens[user])
	}
	req.Header.Set("Idempotency-Key", key)
	w := httptest.NewRecorder()
	f.router.ServeHTTP(w, req)
	return w
}

func sharingHTTPResponse[T any](t *testing.T, w *httptest.ResponseRecorder, status int, fields ...string) T {
	t.Helper()
	result := shoppingResponse[T](t, w, status)
	if w.Header().Get("Cache-Control") != "no-store" || len(w.Header().Values("Set-Cookie")) != 0 {
		t.Fatal("Ответ обмена кешируется или меняет cookie")
	}
	if !strings.HasPrefix(w.Header().Get("Content-Type"), "application/json") {
		t.Fatal("Ответ обмена не является JSON")
	}
	var dto map[string]json.RawMessage
	if err := json.Unmarshal(w.Body.Bytes(), &dto); err != nil || len(dto) != len(fields) {
		t.Fatalf("Лишние/отсутствующие поля ответа: %s", w.Body.String())
	}
	for _, field := range fields {
		if _, ok := dto[field]; !ok {
			t.Fatalf("Отсутствует поле %s", field)
		}
	}
	return result
}

func sharingHTTPError(t *testing.T, w *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	fields := []string{"code", "message"}
	if code == "validation_failed" {
		fields = append(fields, "fieldErrors")
	}
	result := sharingHTTPResponse[shopping.ErrorResponse](t, w, status, fields...)
	if result.Code != code || result.Message == "" {
		t.Fatalf("Неверная ошибка: %s", w.Body.String())
	}
}

func TestSharingHTTPImmutableSnapshotAndRepeatedCopies(t *testing.T) {
	for _, preserve := range []bool{false, true} {
		t.Run(fmt.Sprint(preserve), func(t *testing.T) {
			t.Parallel()
			f := newSharingHTTPFixture(t)
			for _, sequence := range []string{"shopping_lists_id_seq", "shopping_items_id_seq", "shopping_list_shares_id_seq"} {
				migrationExec(t, f.pool, "SELECT setval('"+sequence+"', 9007199254740992)")
			}
			list := f.list(t)
			original := f.items(t, list)
			path := shoppingPath(list.ID) + "/shares"
			body := fmt.Sprintf(`{"token":%q,"preserveMarks":%t}`, sharingToken(1), preserve)
			created := f.request(f.author, "POST", path, body, "http-create-share-0001")
			share := sharingHTTPResponse[shopping.Share](t, created, 201, "id", "createdAt", "preserveMarks")
			if share.ID <= 9007199254740992 || share.CreatedAt.IsZero() || share.PreserveMarks != preserve {
				t.Fatal("Метаданные снимка потеряли строковый BIGINT, время или режим отметок")
			}
			reordered := fmt.Sprintf(`{ "preserveMarks": %t, "token": %q }`, preserve, sharingToken(1))
			replayed := f.request(f.author, "POST", path, reordered, "http-create-share-0001")
			sharingHTTPResponse[shopping.Share](t, replayed, 201, "id", "createdAt", "preserveMarks")
			if replayed.Body.String() != created.Body.String() {
				t.Fatal("Повтор создания изменил исходный ответ 201")
			}
			sharingHTTPError(t, f.request(f.author, "POST", path, body, "http-create-share-0002"), 409, "share_token_conflict")
			changed := fmt.Sprintf(`{"token":%q,"preserveMarks":%t}`, sharingToken(1), !preserve)
			sharingHTTPError(t, f.request(f.author, "POST", path, changed, "http-create-share-0001"), 409, "idempotency_conflict")
			shoppingStatus(t, f.request(f.author, "PATCH", shoppingPath(list.ID), `{"name":"Изменённый оригинал"}`, ""), 200)
			shoppingStatus(t, f.request(f.author, "DELETE", shoppingPath(list.ID)+"/items", "", ""), 204)
			shoppingStatus(t, f.request(f.author, "DELETE", shoppingPath(list.ID), "", ""), 204)
			afterDelete := f.request(f.author, "POST", path, body, "http-create-share-0001")
			sharingHTTPResponse[shopping.Share](t, afterDelete, 201, "id", "createdAt", "preserveMarks")
			if afterDelete.Body.String() != created.Body.String() {
				t.Fatal("Удаление оригинала мешает повтору готового снимка")
			}

			importPath := "/shopping-list-shares/import"
			importBody := fmt.Sprintf(`{"token":%q}`, sharingToken(1))
			firstResponse := f.request(f.recipient, "POST", importPath, importBody, "http-import-share-0001")
			first := sharingHTTPResponse[shopping.List](t, firstResponse, 201, "id", "name", "createdAt", "updatedAt")
			if first.Name != list.Name || first.ID == list.ID || first.CreatedAt.IsZero() || first.UpdatedAt.IsZero() {
				t.Fatal("Копия не получила исходное имя и собственные метаданные")
			}
			assertSharingItems(t, f.sharingFixture, f.recipient, first, original, preserve)
			// A lost HTTP response reuses the action; editing the copy must not alter that response.
			shoppingStatus(t, f.request(f.recipient, "PATCH", shoppingPath(first.ID), `{"name":"Моя копия"}`, ""), 200)
			shoppingStatus(t, f.request(f.recipient, "DELETE", shoppingPath(first.ID)+"/items", "", ""), 204)
			again := f.request(f.recipient, "POST", importPath, importBody, "http-import-share-0001")
			sharingHTTPResponse[shopping.List](t, again, 201, "id", "name", "createdAt", "updatedAt")
			if again.Body.String() != firstResponse.Body.String() {
				t.Fatal("Повтор импорта потерял исходный ответ")
			}
			current := shoppingResponse[shopping.List](t, f.request(f.recipient, "GET", shoppingPath(first.ID), "", ""), 200)
			if current.Name != "Моя копия" {
				t.Fatal("Повтор импорта перезаписал изменённую копию")
			}
			second := sharingHTTPResponse[shopping.List](t,
				f.request(f.recipient, "POST", importPath, importBody, "http-import-share-0002"),
				201, "id", "name", "createdAt", "updatedAt")
			if second.ID == first.ID {
				t.Fatal("Новое подтверждение не создало отдельную копию")
			}
			assertSharingItems(t, f.sharingFixture, f.recipient, second, original, preserve)
			shoppingStatus(t, f.request(f.author, "GET", shoppingPath(second.ID), "", ""), 404)
			shoppingStatus(t, f.request(f.author, "DELETE", shoppingPath(second.ID), "", ""), 404)
			otherToken := fmt.Sprintf(`{"token":%q}`, sharingToken(2))
			sharingHTTPError(t, f.request(f.recipient, "POST", importPath, otherToken, "http-import-share-0001"), 409, "idempotency_conflict")
			shoppingStatus(t, f.request(f.recipient, "DELETE", shoppingPath(first.ID), "", ""), 204)
			sharingHTTPError(t, f.request(f.recipient, "POST", importPath, importBody, "http-import-share-0001"), 409, "resource_deleted")
			third := sharingHTTPResponse[shopping.List](t,
				f.request(f.recipient, "POST", importPath, importBody, "http-import-share-0003"),
				201, "id", "name", "createdAt", "updatedAt")
			assertSharingItems(t, f.sharingFixture, f.recipient, third, original, preserve)
			self := sharingHTTPResponse[shopping.List](t,
				f.request(f.author, "POST", importPath, importBody, "http-import-share-0001"),
				201, "id", "name", "createdAt", "updatedAt")
			assertSharingItems(t, f.sharingFixture, f.author, self, original, preserve)
			for _, secret := range []string{sharingToken(1), f.tokens[f.author], f.tokens[f.recipient], "http-import-share-0001"} {
				if strings.Contains(f.logs.String(), secret) {
					t.Fatal("HTTP-журнал раскрывает секреты обмена или авторизации")
				}
			}
		})
	}
}

func TestSharingHTTPAccessValidationAndEmptyList(t *testing.T) {
	t.Parallel()
	f := newSharingHTTPFixture(t)
	list := f.list(t)
	path := shoppingPath(list.ID) + "/shares"
	body := fmt.Sprintf(`{"token":%q,"preserveMarks":false}`, sharingToken(1))
	before := migrationScalar(t, f.pool, sharingDataState)
	foreign := f.request(f.recipient, "POST", path, body, "http-forbidden-share-0001")
	missing := f.request(f.recipient, "POST", shoppingPath(9223372036854775807)+"/shares", body, "http-forbidden-share-0001")
	sharingHTTPError(t, foreign, 404, "not_found")
	sharingHTTPError(t, missing, 404, "not_found")
	if foreign.Body.String() != missing.Body.String() {
		t.Fatal("Ответ раскрывает существование чужого списка")
	}
	for _, route := range []struct{ path, body string }{
		{path, body}, {"/shopping-list-shares/import", fmt.Sprintf(`{"token":%q}`, sharingToken(1))},
	} {
		sharingHTTPError(t, f.request(0, "POST", route.path, route.body, "http-guest-share-0001"), 401, "unauthorized")
		sharingHTTPError(t, f.request(f.author, "POST", route.path, route.body, ""), 400, "validation_failed")
		sharingHTTPError(t, f.request(f.author, "POST", route.path, route.body+"{}", "http-invalid-share-0001"), 400, "invalid_request")
		sharingHTTPError(t, f.request(f.author, "POST", route.path,
			strings.Repeat(" ", shopping.MaxJSONBodyBytes+1), "http-invalid-share-0001"), 413, "body_too_large")
	}
	importBody := fmt.Sprintf(`{"token":%q}`, sharingToken(1))
	sharingHTTPError(t,
		f.request(f.recipient, "POST", "/shopping-list-shares/import", importBody, "http-unknown-share-0001"),
		404, "not_found")
	if migrationScalar(t, f.pool, sharingDataState) != before {
		t.Fatal("Отклонённые HTTP-запросы изменили данные или создали ключи повторов")
	}
	sharingHTTPResponse[shopping.Share](t, f.request(f.author, "POST", path, body, "http-empty-share-0001"),
		201, "id", "createdAt", "preserveMarks")
	copy := sharingHTTPResponse[shopping.List](t,
		f.request(f.recipient, "POST", "/shopping-list-shares/import", importBody, "http-empty-import-0001"),
		201, "id", "name", "createdAt", "updatedAt")
	w := f.request(f.recipient, "GET", shoppingPath(copy.ID)+"/items", "", "")
	items := shoppingResponse[[]shopping.Item](t, w, 200)
	if items == nil || len(items) != 0 || w.Body.String() != "[]" {
		t.Fatal("Копия пустого снимка не вернула []")
	}
}

func TestSharingHTTPStorageFailureRollsBackAndHidesSecrets(t *testing.T) {
	for _, importMode := range []bool{false, true} {
		t.Run(fmt.Sprint(importMode), func(t *testing.T) {
			t.Parallel()
			f := newSharingHTTPFixture(t)
			list := f.list(t)
			original := f.items(t, list)
			path := shoppingPath(list.ID) + "/shares"
			body := fmt.Sprintf(`{"token":%q,"preserveMarks":true}`, sharingToken(1))
			if importMode {
				f.share(t, list, 1, true)
				path, body = "/shopping-list-shares/import", fmt.Sprintf(`{"token":%q}`, sharingToken(1))
			}
			before := migrationScalar(t, f.pool, sharingDataState)
			migrationExec(t, f.pool, `CREATE FUNCTION fail_share_http() RETURNS trigger LANGUAGE plpgsql AS $$
				BEGIN RAISE EXCEPTION 'SQL password=dependency-http-secret'; END $$`)
			migrationExec(t, f.pool, `CREATE TRIGGER fail_share_http BEFORE UPDATE ON shopping_idempotency
				FOR EACH ROW WHEN (NEW.response IS NOT NULL) EXECUTE FUNCTION fail_share_http()`)
			user := f.author
			if importMode {
				user = f.recipient
			}
			w := f.request(user, "POST", path, body, "http-failed-share-0001")
			sharingHTTPError(t, w, 500, "internal_error")
			if migrationScalar(t, f.pool, sharingDataState) != before {
				t.Fatal("HTTP 500 оставил частичную операцию")
			}
			for _, secret := range []string{"dependency-http-secret", sharingToken(1), f.tokens[f.author], f.tokens[f.recipient]} {
				if strings.Contains(f.logs.String()+w.Body.String(), secret) {
					t.Fatal("HTTP 500 или журнал раскрывает секреты")
				}
			}
			if !strings.Contains(f.logs.String(), "shopping share") {
				t.Fatal("Ошибка зависимости потеряла безопасную диагностику")
			}
			migrationExec(t, f.pool, `DROP TRIGGER fail_share_http ON shopping_idempotency`)
			retry := f.request(user, "POST", path, body, "http-failed-share-0001")
			if importMode {
				copy := sharingHTTPResponse[shopping.List](t, retry, 201, "id", "name", "createdAt", "updatedAt")
				assertSharingItems(t, f.sharingFixture, f.recipient, copy, original, true)
			} else {
				share := sharingHTTPResponse[shopping.Share](t, retry, 201, "id", "createdAt", "preserveMarks")
				if !share.PreserveMarks {
					t.Fatal("Повтор после 500 потерял выбранные отметки")
				}
			}
			if got := migrationScalar(t, f.pool, `SELECT count(*)::text FROM shopping_list_share_items`); got != fmt.Sprint(len(original)) {
				t.Fatal("Повтор HTTP создал неполный снимок")
			}
			if !reflect.DeepEqual(original, f.itemsFromSource(t, list.ID)) {
				t.Fatal("Ошибка/повтор обмена меняет исходные позиции")
			}
		})
	}
}

func (f *sharingHTTPFixture) itemsFromSource(t *testing.T, list int64) []shopping.Item {
	t.Helper()
	return shoppingResponse[[]shopping.Item](t, f.request(f.author, "GET", shoppingPath(list)+"/items", "", ""), 200)
}
