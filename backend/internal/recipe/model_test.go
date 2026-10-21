package recipe

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"
)

func jsonFields(t *testing.T, value any, keys ...string) map[string]json.RawMessage {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatal(err)
	}
	if len(fields) != len(keys) {
		t.Fatalf("Неверный состав JSON: %s", data)
	}
	for _, key := range keys {
		if _, exists := fields[key]; !exists {
			t.Fatalf("Отсутствует обязательное поле %s: %s", key, data)
		}
	}
	return fields
}

func TestRecipeDTOJSONPreservesIDDecimalAndPlainText(t *testing.T) {
	created := time.Date(2026, time.October, 21, 10, 0, 0, 0, time.UTC)
	recipe := Recipe{
		Summary: Summary{
			ID: 9223372036854775807, Title: "Омлет", CookingTimeMinutes: 15,
			CategoryCode: "main-dishes", CreatedAt: created, UpdatedAt: created,
		},
		Ingredients: []Ingredient{
			{Position: 1, Name: "Яйца", Quantity: quantityPointer("999999999.999")},
			{Position: 2, Name: "Соль", Unit: "по вкусу"},
		},
		Steps: []Step{{Position: 1, Text: "Взбить яйца.\n\tПриготовить. <b>Текст</b>"}},
	}
	fields := jsonFields(t, recipe,
		"id", "title", "cookingTimeMinutes", "categoryCode", "createdAt", "updatedAt", "ingredients", "steps")
	if string(fields["id"]) != `"9223372036854775807"` || string(fields["cookingTimeMinutes"]) != "15" {
		t.Fatal("ID должен быть строкой, а время — целым числом")
	}
	for _, field := range []string{"createdAt", "updatedAt"} {
		if string(fields[field]) != `"2026-10-21T10:00:00Z"` {
			t.Fatalf("Некорректный формат UTC поля %s", field)
		}
	}
	first := jsonFields(t, recipe.Ingredients[0], "position", "name", "quantity", "unit")
	if string(first["quantity"]) != `"999999999.999"` || string(first["position"]) != "1" ||
		string(first["unit"]) != `""` {
		t.Fatal("JSON теряет точность количества, позицию или пустую единицу")
	}
	second := jsonFields(t, recipe.Ingredients[1], "position", "name", "quantity", "unit")
	if string(second["quantity"]) != "null" {
		t.Fatal("Отсутствующее количество должно быть явным null")
	}
	jsonFields(t, recipe.Steps[0], "position", "text")
	data, err := json.Marshal(recipe)
	if err != nil {
		t.Fatal(err)
	}
	var replay Recipe
	if err := json.Unmarshal(data, &replay); err != nil || !reflect.DeepEqual(replay, recipe) {
		t.Fatal("DTO изменяется при сохранении и чтении replay", err)
	}
	jsonFields(t, recipe.Summary, "id", "title", "cookingTimeMinutes", "categoryCode", "createdAt", "updatedAt")
}

func TestSaveRequestJSONDefaultsAndServerFields(t *testing.T) {
	var req SaveRequest
	err := json.Unmarshal([]byte(`{
		"title":"Омлет", "cookingTimeMinutes":15, "categoryCode":"main-dishes",
		"ingredients":[{"name":"Яйца","quantity":"2.000"},{"name":"Соль"}],
		"steps":[{"text":"Приготовить"}]
	}`), &req)
	if err != nil {
		t.Fatal(err)
	}
	normalized, err := normalizeSaveRequest(req)
	if err != nil {
		t.Fatal(err)
	}
	if normalized.Ingredients[1].Quantity != nil || normalized.Ingredients[1].Unit != "" {
		t.Fatal("Пропущенные количество и единица должны нормализоваться в null и пустую строку")
	}
	jsonFields(t, normalized, "title", "cookingTimeMinutes", "categoryCode", "ingredients", "steps")
	jsonFields(t, normalized.Ingredients[0], "name", "quantity", "unit")
	jsonFields(t, normalized.Steps[0], "text")
}

func TestSaveRequestRejectsWrongJSONTypes(t *testing.T) {
	for _, body := range []string{
		`{"title":123}`, `{"categoryCode":[]}`, `{"cookingTimeMinutes":"15"}`,
		`{"cookingTimeMinutes":5.5}`, `{"cookingTimeMinutes":5.0}`, `{"cookingTimeMinutes":true}`,
		`{"cookingTimeMinutes":9223372036854775808}`, `{"ingredients":{}}`, `{"steps":"текст"}`,
		`{"ingredients":[{"name":1}]}`, `{"ingredients":[{"quantity":0.125}]}`,
		`{"ingredients":[{"quantity":true}]}`, `{"ingredients":[{"quantity":[]}]}`,
		`{"ingredients":[{"unit":2}]}`, `{"steps":[{"text":123}]}`,
	} {
		t.Run(body, func(t *testing.T) {
			var req SaveRequest
			if err := json.Unmarshal([]byte(body), &req); err == nil {
				t.Fatal("Принят неправильный тип поля")
			}
		})
	}
	// Неизвестные/дублирующиеся поля и запрещённый null проверит строгий HTTP-декодер.
}

func TestErrorResponseJSON(t *testing.T) {
	jsonFields(t, ErrorResponse{Code: "not_found", Message: "Ресурс не найден"}, "code", "message")
	jsonFields(t, ErrorResponse{
		Code: "validation_failed", Message: "Проверьте поля запроса",
		FieldErrors: map[string]string{"steps.0.text": "Недопустимое значение"},
	}, "code", "message", "fieldErrors")
}

func TestCategoriesContractAndIndependentCopy(t *testing.T) {
	want := []Category{
		{Code: "soups", Name: "Супы"}, {Code: "salads", Name: "Салаты"},
		{Code: "starters", Name: "Закуски"}, {Code: "main-dishes", Name: "Основные блюда"},
		{Code: "side-dishes", Name: "Гарниры"}, {Code: "baking", Name: "Выпечка"},
		{Code: "desserts", Name: "Десерты"}, {Code: "sauces", Name: "Соусы"},
		{Code: "drinks", Name: "Напитки"}, {Code: "other", Name: "Другое"},
	}
	got := Categories()
	if !reflect.DeepEqual(got, want) {
		t.Fatal("Изменились коды, подписи или порядок категорий")
	}
	jsonFields(t, got[0], "code", "name")
	got[0] = Category{Code: "invalid", Name: "Изменено"}
	if !reflect.DeepEqual(Categories(), want) || validCategory("invalid") || !validCategory("soups") {
		t.Fatal("Изменение результата испортило справочник или валидацию")
	}
}
