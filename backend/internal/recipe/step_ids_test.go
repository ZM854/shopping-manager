package recipe

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"reflect"
	"strings"
	"testing"
)

func TestStepIDValidationAndIndependentCopy(t *testing.T) {
	id := "9223372036854775807"
	req := serviceRequest()
	req.Steps[0].ID = &id
	got, err := normalizeSaveRequest(req)
	if err != nil || got.Steps[0].ID == nil || *got.Steps[0].ID != id {
		t.Fatal("Потерян строковый BIGINT шага", err)
	}
	*got.Steps[0].ID = "1"
	if id != "9223372036854775807" {
		t.Fatal("Нормализация изменила исходный указатель ID")
	}
	for _, value := range []string{"", "0", "-1", "01", "1.0", "1e2", " 1", "9223372036854775808"} {
		req.Steps[0].ID = &value
		result, err := normalizeSaveRequest(req)
		assertValidationField(t, err, "steps.0.id")
		if !reflect.DeepEqual(result, SaveRequest{}) {
			t.Fatal("Невалидный ID вернул частичное тело")
		}
	}
	duplicate := "5"
	req.Steps = []StepRequest{{ID: &duplicate, Text: "Один"}, {ID: &duplicate, Text: "Два"}}
	_, err = normalizeSaveRequest(req)
	assertValidationField(t, err, "steps.1.id")
	_, err = NewService(nil).CreateRecipe(context.Background(), 7, "service-recipe-0001", serviceRequestWithStepID("5"))
	assertValidationField(t, err, "steps.0.id")
}

func serviceRequestWithStepID(id string) SaveRequest {
	req := serviceRequest()
	req.Steps[0].ID = &id
	return req
}

func TestStepIDHTTPBoundary(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	withID := strings.Replace(handlerBody, `"steps":[{`, `"steps":[{"id":"9223372036854775807",`, 1)
	s := &handlerService{}
	engine := handlerEngine(s, int64(7), log)
	if w := handlerRequest(engine, "POST", "/api/recipes", withID, -1); w.Code != 400 || s.called != "" {
		t.Fatal("POST принял серверный ID шага")
	}
	w := handlerRequest(engine, "PUT", "/api/recipes/11", withID, -1)
	if w.Code != 200 || s.req.Steps[0].ID == nil || *s.req.Steps[0].ID != "9223372036854775807" {
		t.Fatal("PUT не сохранил точный строковый ID")
	}
	for _, raw := range []string{`null`, `1`, `true`, `[]`, `{}`} {
		s.called = ""
		body := strings.Replace(withID, `"9223372036854775807"`, raw, 1)
		if w := handlerRequest(engine, "PUT", "/api/recipes/11", body, -1); w.Code != 400 || s.called != "" {
			t.Fatal("PUT принял неверный тип или null ID")
		}
	}
	for _, field := range []string{`"id":"2",`, `"\u0069d":"2",`, `"ID":"2",`, `"position":1,`} {
		body := strings.Replace(withID, `"steps":[{`, `"steps":[{`+field, 1)
		if w := handlerRequest(engine, "PUT", "/api/recipes/11", body, -1); w.Code != 400 {
			t.Fatal("PUT принял дубликат/неизвестное поле шага")
		}
	}
}

func TestReplayPreservesOriginalStepIdentity(t *testing.T) {
	id := int64(11)
	for _, tc := range []struct {
		name      string
		stepID    int64
		resource  int64
		wantError bool
	}{
		{"current", 99, id, false},
		{"missing ID", 0, id, true},
		{"wrong recipe", 99, 12, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			original := Recipe{Summary: Summary{ID: tc.resource, Title: "До изменения"}, Steps: []Step{{ID: tc.stepID, Position: 1, Text: "Исходный шаг"}}}
			body, err := json.Marshal(original)
			if err != nil {
				t.Fatal(err)
			}
			tx := newServiceTransaction(t)
			tx.replay = Replay{ResourceID: &id, Response: body}
			got, err := NewService(serviceStore{tx: tx}).CreateRecipe(context.Background(), 7, "service-recipe-0001", serviceRequest())
			if tc.wantError {
				if err == nil || !reflect.DeepEqual(got, Recipe{}) {
					t.Fatal("Повреждённый replay выдан за успех")
				}
				return
			}
			if err != nil || !reflect.DeepEqual(got, original) {
				t.Fatal("Replay изменил исходный снимок", err)
			}
			encoded, err := json.Marshal(got)
			if err != nil || string(encoded) != string(body) {
				t.Fatal("Replay изменил поля исходного снимка")
			}
			if !reflect.DeepEqual(tx.calls, []string{"claim", "get"}) {
				t.Fatal("Replay записал рецепт повторно или пропустил проверку доступа")
			}
		})
	}
}
