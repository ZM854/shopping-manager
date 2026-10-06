package shopping

import (
	"encoding/json"
	"testing"
)

func TestItemJSONPreservesIDAndDecimal(t *testing.T) {
	quantity := "0.125"
	item := Item{ID: 9223372036854775807, ListID: 9007199254740993, Name: "Молоко", Quantity: &quantity}
	data, err := json.Marshal(item)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatal(err)
	}
	if fields["id"] != "9223372036854775807" || fields["listId"] != "9007199254740993" || fields["quantity"] != quantity {
		t.Fatalf("JSON теряет точность: %s", data)
	}
	item.Quantity = nil
	data, err = json.Marshal(item)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatal(err)
	}
	if value, exists := fields["quantity"]; !exists || value != nil {
		t.Fatalf("Неуказанное количество должно быть явным null: %s", data)
	}
}

func TestPatchPreservesPresenceNullAndFalse(t *testing.T) {
	for _, body := range []string{`{}`, `{"quantity":null}`, `{"quantity":"0.125"}`, `{"isMarked":false}`} {
		t.Run(body, func(t *testing.T) {
			var req UpdateItemRequest
			if err := json.Unmarshal([]byte(body), &req); err != nil {
				t.Fatal(err)
			}
			data, err := json.Marshal(req)
			if err != nil {
				t.Fatal(err)
			}
			if string(data) != body {
				t.Fatalf("PATCH меняет семантику: %s → %s", body, data)
			}
		})
	}
}

func TestPatchRejectsWrongJSONTypes(t *testing.T) {
	for _, body := range []string{`{"quantity":0.125}`, `{"isMarked":"false"}`, `{"name":17}`, `{"unit":[]}`} {
		var req UpdateItemRequest
		if err := json.Unmarshal([]byte(body), &req); err == nil {
			t.Errorf("Ожидался отказ для %s", body)
		}
	}
}
