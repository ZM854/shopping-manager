package shopping

import (
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

var sharingHTTPPaths = []string{"/shopping-lists/1/shares", "/shopping-list-shares/import"}

func sharingHTTPBody(importMode bool) string {
	body := `{"token":"` + unitShareToken(1) + `"`
	if !importMode {
		body += `,"preserveMarks":false`
	}
	return body + `}`
}

func TestSharingHTTPStrictJSON(t *testing.T) {
	r := handlerEngine(NewService(&forbiddenStore{t: t}), int64(7))
	for _, path := range sharingHTTPPaths {
		for _, body := range []string{
			"", "null", "[]", "1", "true", "{", `{} {}`, `{} false`,
			`{"token":null}`, `{"token":12}`, `{"token":true}`, `{"token":[]}`,
			`{"Token":"a"}`, `{"token":"a","token":"b"}`, `{"to\u006ben":"a","token":"b"}`,
			`{"token":"a","user_id":"7"}`, `{"token":"a","listId":"1"}`,
			`{"token":"a","owner_id":"7"}`, `{"token":"a","shareId":"1"}`,
			"{\"token\":\"\xff\"}",
		} {
			t.Run(path+body, func(t *testing.T) {
				w := handlerRequest(r, "POST", path, body, "sharing-http-action-001")
				assertSharingHTTPError(t, w, 400, "invalid_request", "")
			})
		}
	}
	for _, body := range []string{
		`{"token":"a","preserveMarks":null}`, `{"token":"a","preserveMarks":"false"}`,
		`{"token":"a","preserveMarks":0}`, `{"token":"a","preserveMarks":[]}`,
		`{"preserveMarks":true,"preserveMarks":false}`, `{"PreserveMarks":true}`,
	} {
		w := handlerRequest(r, "POST", sharingHTTPPaths[0], body, "sharing-http-action-001")
		assertSharingHTTPError(t, w, 400, "invalid_request", "")
	}
	w := handlerRequest(r, "POST", sharingHTTPPaths[1], sharingHTTPBody(false), "sharing-http-action-001")
	assertSharingHTTPError(t, w, 400, "invalid_request", "")
}

func TestSharingHTTPValidationBeforeStorage(t *testing.T) {
	r := handlerEngine(NewService(&forbiddenStore{t: t}), int64(7))
	for i, path := range sharingHTTPPaths {
		body := sharingHTTPBody(i == 1)
		for _, key := range []string{"", "short", strings.Repeat("x", 129), strings.Repeat("!", 16), "ключ-действия-00001"} {
			w := handlerRequest(r, "POST", path, body, key)
			assertSharingHTTPError(t, w, 400, "validation_failed", "Idempotency-Key")
		}
		for _, token := range []string{
			"", unitShareToken(1)[1:], unitShareToken(1) + "=", " " + unitShareToken(1),
			unitShareToken(1)[:42] + "B", "+" + unitShareToken(1)[1:], "/" + unitShareToken(1)[1:],
		} {
			bad := strings.Replace(body, unitShareToken(1), token, 1)
			w := handlerRequest(r, "POST", path, bad, "sharing-http-action-001")
			assertSharingHTTPError(t, w, 400, "validation_failed", "token")
		}
		w := handlerRequest(r, "POST", path, `{}`, "sharing-http-action-001")
		assertSharingHTTPError(t, w, 400, "validation_failed", "token")
	}
	w := handlerRequest(r, "POST", sharingHTTPPaths[0], sharingHTTPBody(true), "sharing-http-action-001")
	assertSharingHTTPError(t, w, 400, "validation_failed", "preserveMarks")
	for _, id := range []string{"0", "-1", "01", "+1", "9223372036854775808", "abc"} {
		w := handlerRequest(r, "POST", "/shopping-lists/"+id+"/shares", sharingHTTPBody(false), "sharing-http-action-001")
		assertSharingHTTPError(t, w, 400, "validation_failed", "id")
	}
	for _, user := range []any{nil, "7", int64(0), int64(-1)} {
		for _, path := range sharingHTTPPaths {
			w := handlerRequest(handlerEngine(nil, user), "POST", path, "invalid JSON", "")
			assertSharingHTTPError(t, w, 401, "unauthorized", "")
		}
	}
}

func TestSharingHTTPBodyLimitAndReadFailures(t *testing.T) {
	for i, path := range sharingHTTPPaths {
		for _, unknownLength := range []bool{false, true} {
			body := sharingHTTPBody(i == 1)
			body += strings.Repeat(" ", MaxJSONBodyBytes-len(body))
			for _, extra := range []string{"", " "} {
				tx := &shareTestTransaction{}
				r := handlerEngine(NewService(transactionStore{tx: tx}), int64(7))
				req := httptest.NewRequest("POST", "/api"+path, strings.NewReader(body+extra))
				req.Header.Set("Idempotency-Key", "sharing-http-action-001")
				if unknownLength {
					req.ContentLength = -1
					req.TransferEncoding = []string{"chunked"}
				}
				w := httptest.NewRecorder()
				r.ServeHTTP(w, req)
				if extra != "" {
					assertSharingHTTPError(t, w, 413, "body_too_large", "")
					if tx.saved || tx.created {
						t.Fatal("Слишком большое тело достигло записи в БД")
					}
				} else if w.Code != 201 || !tx.saved {
					t.Fatalf("Тело ровно 65536 байт отклонено: %d %s", w.Code, w.Body.String())
				}
			}
		}
		req := httptest.NewRequest("POST", "/api"+path, nil)
		req.Body = failingBody{}
		w := httptest.NewRecorder()
		handlerEngine(nil, int64(7)).ServeHTTP(w, req)
		assertSharingHTTPError(t, w, 400, "invalid_request", "")
	}
}

func TestSharingHTTPErrorMappingAndPrivateLogs(t *testing.T) {
	for i, path := range sharingHTTPPaths {
		for _, tc := range []struct {
			err    error
			status int
			code   string
		}{
			{ErrNotFound, 404, "not_found"},
			{ErrIdempotencyConflict, 409, "idempotency_conflict"},
			{ErrResourceDeleted, 409, "resource_deleted"},
			{errors.New("SQL dependency-secret"), 500, "internal_error"},
		} {
			var logs bytes.Buffer
			r := gin.New()
			group := r.Group("/api", func(c *gin.Context) { c.Set("userID", int64(7)) })
			NewHandler(NewService(transactionStore{beginErr: tc.err}),
				slog.New(slog.NewJSONHandler(&logs, nil))).Register(group)
			w := handlerRequest(r, "POST", path, sharingHTTPBody(i == 1), "sharing-http-action-001")
			assertSharingHTTPError(t, w, tc.status, tc.code, "")
			for _, secret := range []string{"dependency-secret", unitShareToken(1), "sharing-http-action-001"} {
				if strings.Contains(logs.String()+w.Body.String(), secret) {
					t.Fatal("Ответ или журнал раскрывает секрет запроса/зависимости")
				}
			}
			if tc.status == 500 && !strings.Contains(logs.String(), "shopping share") {
				t.Fatal("Безопасная диагностика ошибки не записана")
			}
		}
	}
	r := handlerEngine(NewService(transactionStore{beginErr: ErrShareTokenConflict}), int64(7))
	w := handlerRequest(r, "POST", sharingHTTPPaths[0], sharingHTTPBody(false), "sharing-http-action-001")
	assertSharingHTTPError(t, w, 409, "share_token_conflict", "")
}

func assertSharingHTTPError(t *testing.T, w *httptest.ResponseRecorder, status int, code, field string) {
	t.Helper()
	var result ErrorResponse
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil || w.Code != status || result.Code != code || result.Message == "" {
		t.Fatalf("Неверная ошибка HTTP: %d %s", w.Code, w.Body.String())
	}
	if field != "" && result.FieldErrors[field] == "" {
		t.Fatalf("Отсутствует ошибка поля %s: %s", field, w.Body.String())
	}
}
