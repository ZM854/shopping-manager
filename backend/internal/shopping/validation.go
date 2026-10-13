package shopping

import (
	"errors"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

var (
	ErrNotFound            = errors.New("shopping resource not found")
	ErrIdempotencyConflict = errors.New("idempotency conflict")
	ErrResourceDeleted     = errors.New("created resource deleted")
	quantityPattern        = regexp.MustCompile(`^(0|[1-9][0-9]{0,8})(\.[0-9]{1,3})?$`)
	keyPattern             = regexp.MustCompile(`^[A-Za-z0-9_-]{16,128}$`)
	idPattern              = regexp.MustCompile(`^[1-9][0-9]{0,18}$`)
)

type ValidationError struct{ Fields map[string]string }

func (e *ValidationError) Error() string { return "invalid shopping fields" }

func invalid(field string) error {
	return &ValidationError{Fields: map[string]string{field: "Недопустимое значение"}}
}

func ParseID(value string) (int64, error) {
	if !idPattern.MatchString(value) {
		return 0, invalid("id")
	}
	id, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0, invalid("id")
	}
	return id, nil
}

func normalizeText(value, field string, max int, required bool) (string, error) {
	if !utf8.ValidString(value) {
		return "", invalid(field)
	}
	value = strings.TrimSpace(value)
	if (required && value == "") || utf8.RuneCountInString(value) > max {
		return "", invalid(field)
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return "", invalid(field)
		}
	}
	return value, nil
}

func normalizeQuantity(value *string) (*string, error) {
	if value == nil {
		return nil, nil
	}
	if !quantityPattern.MatchString(*value) {
		return nil, invalid("quantity")
	}
	normalized := *value
	if strings.Contains(normalized, ".") {
		normalized = strings.TrimRight(strings.TrimRight(normalized, "0"), ".")
	}
	if normalized == "0" {
		return nil, invalid("quantity")
	}
	return &normalized, nil
}

func normalizeCreateItem(req CreateItemRequest) (CreateItemRequest, error) {
	var err error
	req.Name, err = normalizeText(req.Name, "name", MaxItemNameLength, true)
	if err != nil {
		return req, err
	}
	req.Unit, err = normalizeText(req.Unit, "unit", MaxUnitLength, false)
	if err != nil {
		return req, err
	}
	req.Quantity, err = normalizeQuantity(req.Quantity)
	return req, err
}

func normalizeUpdateItem(req UpdateItemRequest) (UpdateItemRequest, error) {
	if !req.Name.Present && !req.Quantity.Present && !req.Unit.Present && !req.IsMarked.Present {
		return req, invalid("body")
	}
	for _, field := range []struct {
		value    *PatchField[string]
		name     string
		max      int
		required bool
	}{
		{&req.Name, "name", MaxItemNameLength, true}, {&req.Unit, "unit", MaxUnitLength, false},
	} {
		if !field.value.Present {
			continue
		}
		if field.value.Value == nil {
			return req, invalid(field.name)
		}
		value, err := normalizeText(*field.value.Value, field.name, field.max, field.required)
		if err != nil {
			return req, err
		}
		field.value.Value = &value
	}
	if req.IsMarked.Present && req.IsMarked.Value == nil {
		return req, invalid("isMarked")
	}
	if req.Quantity.Present {
		value, err := normalizeQuantity(req.Quantity.Value)
		if err != nil {
			return req, err
		}
		req.Quantity.Value = value
	}
	return req, nil
}
