package recipe

import (
	"encoding/json"
	"errors"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"unicode"
)

func quantityPointer(value string) *string { return &value }

func validSaveRequest() SaveRequest {
	return SaveRequest{
		Title: "Омлет", CookingTimeMinutes: 15, CategoryCode: "main-dishes",
		Ingredients: []IngredientRequest{{Name: "Яйца", Quantity: quantityPointer("2"), Unit: "шт."}},
		Steps:       []StepRequest{{Text: "Взбить яйца и приготовить."}},
	}
}

func assertValidationField(t *testing.T, err error, field string) {
	t.Helper()
	var validation *ValidationError
	if !errors.As(err, &validation) || len(validation.Fields) != 1 || validation.Fields[field] == "" {
		t.Fatalf("Ожидалась ошибка поля %s, получена %v", field, err)
	}
}

func TestNormalizeSavePreservesOrderAndDoesNotMutateInput(t *testing.T) {
	req := SaveRequest{
		Title: "\u00a0 Омлет 🍳 \u3000", CookingTimeMinutes: 15, CategoryCode: "main-dishes",
		Ingredients: []IngredientRequest{
			{Name: " Яйца ", Quantity: quantityPointer("2.000"), Unit: " "},
			{Name: "\u2003Соль", Unit: " по вкусу "},
			{Name: "Яйца ", Quantity: quantityPointer("0.100"), Unit: " кг\u00a0"},
		},
		Steps: []StepRequest{
			{Text: "\r\n Первый шаг.\r\n\tОтступ.\rЕщё строка. \r\n"},
			{Text: " Второй шаг. "},
		},
	}
	before, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	got, err := normalizeSaveRequest(req)
	if err != nil {
		t.Fatal(err)
	}
	want := SaveRequest{
		Title: "Омлет 🍳", CookingTimeMinutes: 15, CategoryCode: "main-dishes",
		Ingredients: []IngredientRequest{
			{Name: "Яйца", Quantity: quantityPointer("2")},
			{Name: "Соль", Unit: "по вкусу"},
			{Name: "Яйца", Quantity: quantityPointer("0.1"), Unit: "кг"},
		},
		Steps: []StepRequest{
			{Text: "Первый шаг.\n\tОтступ.\nЕщё строка."}, {Text: "Второй шаг."},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Не сохранились нормализация, порядок или дубликаты: %+v", got)
	}
	again, err := normalizeSaveRequest(got)
	if err != nil || !reflect.DeepEqual(again, got) {
		t.Fatal("Повторная нормализация меняет результат", err)
	}
	got.Ingredients[0].Name = "Изменено"
	*got.Ingredients[0].Quantity = "999"
	got.Steps[0].Text = "Изменено"
	after, err := json.Marshal(req)
	if err != nil || string(after) != string(before) {
		t.Fatal("Нормализация или изменение результата затронули исходную попытку")
	}
}

func TestNormalizeSaveRejectsInvalidFields(t *testing.T) {
	for _, tc := range []struct {
		name, field string
		change      func(*SaveRequest)
	}{
		{"missing title", "title", func(r *SaveRequest) { r.Title = "" }},
		{"blank title", "title", func(r *SaveRequest) { r.Title = " \u00a0\u3000" }},
		{"long title", "title", func(r *SaveRequest) { r.Title = strings.Repeat("🍳", 201) }},
		{"invalid title UTF-8", "title", func(r *SaveRequest) { r.Title = string([]byte{0xff}) }},
		{"missing time", "cookingTimeMinutes", func(r *SaveRequest) { r.CookingTimeMinutes = 0 }},
		{"negative time", "cookingTimeMinutes", func(r *SaveRequest) { r.CookingTimeMinutes = -5 }},
		{"short time", "cookingTimeMinutes", func(r *SaveRequest) { r.CookingTimeMinutes = 4 }},
		{"long time", "cookingTimeMinutes", func(r *SaveRequest) { r.CookingTimeMinutes = 1445 }},
		{"time step", "cookingTimeMinutes", func(r *SaveRequest) { r.CookingTimeMinutes = 7 }},
		{"missing category", "categoryCode", func(r *SaveRequest) { r.CategoryCode = "" }},
		{"unknown category", "categoryCode", func(r *SaveRequest) { r.CategoryCode = "breakfast" }},
		{"uppercase category", "categoryCode", func(r *SaveRequest) { r.CategoryCode = "Soups" }},
		{"padded category", "categoryCode", func(r *SaveRequest) { r.CategoryCode = " soups " }},
		{"invalid category UTF-8", "categoryCode", func(r *SaveRequest) { r.CategoryCode = string([]byte{0xff}) }},
		{"missing ingredients", "ingredients", func(r *SaveRequest) { r.Ingredients = nil }},
		{"empty ingredients", "ingredients", func(r *SaveRequest) { r.Ingredients = []IngredientRequest{} }},
		{"many ingredients", "ingredients", func(r *SaveRequest) {
			r.Ingredients = make([]IngredientRequest, 201)
		}},
		{"missing steps", "steps", func(r *SaveRequest) { r.Steps = nil }},
		{"empty steps", "steps", func(r *SaveRequest) { r.Steps = []StepRequest{} }},
		{"many steps", "steps", func(r *SaveRequest) { r.Steps = make([]StepRequest, 101) }},
		{"empty ingredient", "ingredients.0.name", func(r *SaveRequest) { r.Ingredients[0].Name = "" }},
		{"blank ingredient", "ingredients.0.name", func(r *SaveRequest) { r.Ingredients[0].Name = "\u00a0 " }},
		{"long ingredient", "ingredients.0.name", func(r *SaveRequest) {
			r.Ingredients[0].Name = strings.Repeat("🍳", 201)
		}},
		{"invalid name UTF-8", "ingredients.0.name", func(r *SaveRequest) {
			r.Ingredients[0].Name = string([]byte{0xff})
		}},
		{"zero quantity", "ingredients.0.quantity", func(r *SaveRequest) {
			r.Ingredients[0].Quantity = quantityPointer("0")
		}},
		{"long unit", "ingredients.0.unit", func(r *SaveRequest) {
			r.Ingredients[0].Unit = strings.Repeat("🥄", 33)
		}},
		{"invalid unit UTF-8", "ingredients.0.unit", func(r *SaveRequest) {
			r.Ingredients[0].Unit = string([]byte{0xff})
		}},
		{"empty step", "steps.0.text", func(r *SaveRequest) { r.Steps[0].Text = "" }},
		{"blank step", "steps.0.text", func(r *SaveRequest) { r.Steps[0].Text = " \r\n\t\u00a0" }},
		{"long step", "steps.0.text", func(r *SaveRequest) { r.Steps[0].Text = strings.Repeat("🍳", 20001) }},
		{"invalid step UTF-8", "steps.0.text", func(r *SaveRequest) { r.Steps[0].Text = string([]byte{0xff}) }},
		{"second ingredient", "ingredients.1.name", func(r *SaveRequest) {
			r.Ingredients = append(r.Ingredients, IngredientRequest{})
		}},
		{"second step", "steps.1.text", func(r *SaveRequest) { r.Steps = append(r.Steps, StepRequest{}) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := validSaveRequest()
			tc.change(&req)
			result, err := normalizeSaveRequest(req)
			assertValidationField(t, err, tc.field)
			if !reflect.DeepEqual(result, SaveRequest{}) {
				t.Fatal("При ошибке возвращены частично нормализованные данные")
			}
		})
	}
	_, err := normalizeSaveRequest(SaveRequest{})
	assertValidationField(t, err, "title")
}

func TestNormalizeSaveAcceptsFieldBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*SaveRequest)
	}{
		{"minimum fields", func(r *SaveRequest) {
			r.Title = "Я"
			r.CookingTimeMinutes = 5
			r.Ingredients[0] = IngredientRequest{Name: "Я"}
			r.Steps[0].Text = "Я"
		}},
		{"maximum fields", func(r *SaveRequest) {
			r.Title = "\u00a0" + strings.Repeat("🍳", 200) + "\u3000"
			r.CookingTimeMinutes = 1440
			r.Ingredients[0] = IngredientRequest{
				Name: strings.Repeat("🥛", 200), Quantity: quantityPointer("999999999.999"),
				Unit: strings.Repeat("🥄", 32),
			}
			r.Steps[0].Text = "\r\n" + strings.Repeat("🍳", 20000) + " \r\n"
		}},
		{"quantity without unit", func(r *SaveRequest) { r.Ingredients[0].Unit = "" }},
		{"unit without quantity", func(r *SaveRequest) {
			r.Ingredients[0].Quantity = nil
			r.Ingredients[0].Unit = "по вкусу"
		}},
		{"without quantity and unit", func(r *SaveRequest) {
			r.Ingredients[0].Quantity = nil
			r.Ingredients[0].Unit = "\u00a0 \u3000"
		}},
		{"maximum ingredients", func(r *SaveRequest) {
			for len(r.Ingredients) < 200 {
				r.Ingredients = append(r.Ingredients, r.Ingredients[0])
			}
		}},
		{"maximum steps and total text", func(r *SaveRequest) {
			r.Steps = make([]StepRequest, 100)
			for i := range r.Steps {
				r.Steps[i].Text = strings.Repeat("🍳", 500)
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := validSaveRequest()
			tc.change(&req)
			if _, err := normalizeSaveRequest(req); err != nil {
				t.Fatal(err)
			}
		})
	}
	for _, category := range Categories() {
		req := validSaveRequest()
		req.CategoryCode = category.Code
		if _, err := normalizeSaveRequest(req); err != nil {
			t.Fatalf("Отклонена категория %s: %v", category.Code, err)
		}
	}
}

func TestNormalizeSaveTotalStepTextLimitAfterNormalization(t *testing.T) {
	req := validSaveRequest()
	req.Steps = []StepRequest{
		{Text: strings.Repeat("🍳", 20000)},
		{Text: strings.Repeat("Я", 20000)},
		{Text: "\u00a0" + strings.Repeat("ё", 9998) + "\r\nЯ\u3000"},
	}
	if _, err := normalizeSaveRequest(req); err != nil {
		t.Fatal("Отклонён рецепт с 50000 кодовых точек после нормализации", err)
	}
	req.Steps[2].Text = strings.Repeat("ё", 9999) + "\r\nЯ"
	_, err := normalizeSaveRequest(req)
	assertValidationField(t, err, "steps")
}

func TestNormalizeSaveControlCharacters(t *testing.T) {
	for char := rune(0); char <= '\u009f'; char++ {
		if !unicode.IsControl(char) {
			continue
		}
		t.Run(strconv.QuoteRune(char), func(t *testing.T) {
			for _, field := range []string{"title", "ingredients.0.name", "ingredients.0.unit", "steps.0.text"} {
				req := validSaveRequest()
				value := "Я" + string(char) + "Я"
				switch field {
				case "title":
					req.Title = value
				case "ingredients.0.name":
					req.Ingredients[0].Name = value
				case "ingredients.0.unit":
					req.Ingredients[0].Unit = value
				case "steps.0.text":
					req.Steps[0].Text = value
				}
				_, err := normalizeSaveRequest(req)
				if field == "steps.0.text" && (char == '\n' || char == '\r' || char == '\t') {
					if err != nil {
						t.Fatal("Отклонён допустимый перенос строки/отступ", err)
					}
				} else {
					assertValidationField(t, err, field)
				}
			}
		})
	}
	// Недопустимые управляющие символы не должны исчезать при обрезке краёв.
	for _, value := range []string{"\vНачало", "Конец\f", "\u0085Текст"} {
		req := validSaveRequest()
		req.Steps[0].Text = value
		_, err := normalizeSaveRequest(req)
		assertValidationField(t, err, "steps.0.text")
	}
}

func TestQuantityNormalizationWithoutFloats(t *testing.T) {
	for value, want := range map[string]string{
		"0.001": "0.001", "0.100": "0.1", "2.675": "2.675", "1.000": "1",
		"999999999.999": "999999999.999", "999999999": "999999999",
	} {
		got, err := normalizeQuantity(&value, "ingredients.1.quantity")
		if err != nil || got == nil || *got != want {
			t.Fatalf("Количество %s: %v %v", value, got, err)
		}
	}
	if got, err := normalizeQuantity(nil, "ingredients.1.quantity"); err != nil || got != nil {
		t.Fatal("Отсутствующее количество должно оставаться null")
	}
	for _, value := range []string{
		"", "0", "0.0", "0.000", "0.0001", "-1", "+1", "01", ".5", "1.", "1.0000",
		"1e3", "1,5", "NaN", "Infinity", "-Infinity", " 1", "1 ", "1\n", "١",
		"1000000000", "999999999.9991", "1.0000000000000002", string([]byte{0xff}),
	} {
		_, err := normalizeQuantity(&value, "ingredients.1.quantity")
		assertValidationField(t, err, "ingredients.1.quantity")
	}
}

func TestParseID(t *testing.T) {
	for value, want := range map[string]int64{
		"1": 1, "9007199254740993": 9007199254740993, "9223372036854775807": 9223372036854775807,
	} {
		got, err := ParseID(value)
		if err != nil || got != want {
			t.Fatalf("ID %s: %d %v", value, got, err)
		}
	}
	for _, value := range []string{
		"", "0", "-1", "+1", "01", "1.0", "1e2", " 1", "1 ", "1\n", "١", "9223372036854775808",
		strings.Repeat("9", 100), string([]byte{0xff}),
	} {
		got, err := ParseID(value)
		assertValidationField(t, err, "id")
		if got != 0 {
			t.Fatal("Ошибка ID вернула ненулевой результат")
		}
	}
}

func TestIdempotencyKeyValidation(t *testing.T) {
	for _, value := range []string{strings.Repeat("a", 16), strings.Repeat("Z", 128), "Abc_0123456789-xyz"} {
		if err := validateIdempotencyKey(value); err != nil {
			t.Fatal("Отклонён допустимый ключ", err)
		}
	}
	for _, value := range []string{
		"", strings.Repeat("a", 15), strings.Repeat("a", 129), "recipe key spaces",
		"ключ-повторной-отправки", "recipe-action-01\n", "recipe-action-01,other", "+recipe-action-01",
	} {
		assertValidationField(t, validateIdempotencyKey(value), "Idempotency-Key")
	}
}

func TestValidationErrorDoesNotExposeInput(t *testing.T) {
	req := validSaveRequest()
	req.Ingredients[0].Quantity = quantityPointer("private-recipe-data")
	_, err := normalizeSaveRequest(req)
	assertValidationField(t, err, "ingredients.0.quantity")
	if strings.Contains(err.Error(), "private-recipe-data") {
		t.Fatal("Ошибка содержит исходные данные")
	}
	var validation *ValidationError
	if !errors.As(err, &validation) {
		t.Fatal(err)
	}
	for _, message := range validation.Fields {
		if strings.Contains(message, "private-recipe-data") {
			t.Fatal("Ошибка поля содержит исходные данные")
		}
	}
}
