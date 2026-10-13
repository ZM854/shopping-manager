package shopping

import (
	"errors"
	"strings"
	"testing"
)

func ptr[T any](value T) *T              { return &value }
func field[T any](value T) PatchField[T] { return PatchField[T]{Present: true, Value: &value} }

func TestIDs(t *testing.T) {
	for _, value := range []string{"1", "9007199254740993", "9223372036854775807"} {
		id, err := ParseID(value)
		if err != nil || id <= 0 {
			t.Fatalf("%s: %v", value, err)
		}
	}
	for _, value := range []string{"", "0", "-1", "+1", "01", "1.0", " 1", "1 ", "1e2", "9223372036854775808", "١"} {
		if _, err := ParseID(value); err == nil {
			t.Fatalf("Accepted ID %q", value)
		}
	}
}
func TestDecimalNormalizationWithoutFloats(t *testing.T) {
	for value, expected := range map[string]string{"0.001": "0.001", "0.100": "0.1", "2.675": "2.675", "1.000": "1", "999999999.999": "999999999.999", "999999999": "999999999"} {
		got, err := normalizeQuantity(&value)
		if err != nil || got == nil || *got != expected {
			t.Fatalf("%s: %v %v", value, got, err)
		}
	}
	got, err := normalizeQuantity(nil)
	if got != nil || err != nil {
		t.Fatal("null must remain null")
	}
	for _, value := range []string{"", "0", "0.000", "0.0001", "-1", "+1", "01", ".5", "1.", "1.0000", "1e3", "1,5", "NaN", "Infinity", " 1", "1000000000", "1.0000000000000002"} {
		if _, err := normalizeQuantity(&value); err == nil {
			t.Fatalf("Accepted decimal %q", value)
		}
	}
}
func TestUnicodeTextValidation(t *testing.T) {
	value, err := normalizeText("\u00a0 Молоко 🥛 \u2003", "name", 8, true)
	if err != nil || value != "Молоко 🥛" {
		t.Fatalf("Unicode trim/length: %q %v", value, err)
	}
	for _, value := range []string{"", " \u3000", "a\nb", "a\u0085b", "\x00", strings.Repeat("я", 101), string([]byte{0xff})} {
		_, err := normalizeText(value, "name", 100, true)
		var validation *ValidationError
		if !errors.As(err, &validation) || validation.Fields["name"] == "" || validation.Error() == "" {
			t.Fatalf("Accepted text %q", value)
		}
	}
	for _, size := range []int{MaxListNameLength, MaxItemNameLength, MaxUnitLength} {
		if _, err := normalizeText(strings.Repeat("я", size), "text", size, true); err != nil {
			t.Fatal(err)
		}
	}
	if value, err := normalizeText("  ", "unit", 32, false); value != "" || err != nil {
		t.Fatal("Empty unit rejected")
	}
}
func TestCreateAndPatchValidation(t *testing.T) {
	req, err := normalizeCreateItem(CreateItemRequest{Name: " Молоко ", Unit: " Л ", Quantity: ptr("1.000")})
	if err != nil || req.Name != "Молоко" || req.Unit != "Л" || *req.Quantity != "1" {
		t.Fatalf("Create: %+v %v", req, err)
	}
	patch, err := normalizeUpdateItem(UpdateItemRequest{IsMarked: field(false), Quantity: PatchField[string]{Present: true}, Unit: field("  ")})
	if err != nil || patch.Name.Present || patch.Quantity.Value != nil || *patch.IsMarked.Value || *patch.Unit.Value != "" {
		t.Fatalf("Patch lost absent/null/false: %+v %v", patch, err)
	}
	patch, err = normalizeUpdateItem(UpdateItemRequest{Name: field(" Хлеб "), Quantity: field("0.100")})
	if err != nil || *patch.Name.Value != "Хлеб" || *patch.Quantity.Value != "0.1" {
		t.Fatal("Patch not normalized")
	}
	for _, req := range []CreateItemRequest{{}, {Name: "A", Unit: strings.Repeat("я", 33)}, {Name: "A", Quantity: ptr("0")}} {
		if _, err := normalizeCreateItem(req); err == nil {
			t.Fatalf("Accepted create %+v", req)
		}
	}
	for _, req := range []UpdateItemRequest{
		{}, {Name: PatchField[string]{Present: true}}, {Name: field(" ")},
		{Unit: PatchField[string]{Present: true}}, {Unit: field("a\nb")},
		{IsMarked: PatchField[bool]{Present: true}}, {Quantity: field("0")},
	} {
		if _, err := normalizeUpdateItem(req); err == nil {
			t.Fatalf("Accepted patch %+v", req)
		}
	}
}
