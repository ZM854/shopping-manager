package recipe

import (
	"errors"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

var (
	ErrNotFound            = errors.New("recipe not found")
	ErrIdempotencyConflict = errors.New("recipe idempotency conflict")
	ErrResourceDeleted     = errors.New("created recipe deleted")
	idPattern              = regexp.MustCompile(`^[1-9][0-9]{0,18}$`)
	keyPattern             = regexp.MustCompile(`^[A-Za-z0-9_-]{16,128}$`)
	quantityPattern        = regexp.MustCompile(`^(0|[1-9][0-9]{0,8})(\.[0-9]{1,3})?$`)
	stepLineBreaks         = strings.NewReplacer("\r\n", "\n", "\r", "\n")
)

type ValidationError struct{ Fields map[string]string }

func (e *ValidationError) Error() string { return "invalid recipe fields" }

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

func validateIdempotencyKey(key string) error {
	if !keyPattern.MatchString(key) {
		return invalid("Idempotency-Key")
	}
	return nil
}

func normalizeText(value, field string, max int, required bool) (string, error) {
	if !utf8.ValidString(value) || strings.ContainsFunc(value, unicode.IsControl) {
		return "", invalid(field)
	}
	value = strings.TrimSpace(value)
	if (required && value == "") || utf8.RuneCountInString(value) > max {
		return "", invalid(field)
	}
	return value, nil
}

func normalizeQuantity(value *string, field string) (*string, error) {
	if value == nil {
		return nil, nil
	}
	if !quantityPattern.MatchString(*value) {
		return nil, invalid(field)
	}
	normalized := *value
	if strings.Contains(normalized, ".") {
		normalized = strings.TrimRight(strings.TrimRight(normalized, "0"), ".")
	}
	if normalized == "0" {
		return nil, invalid(field)
	}
	return &normalized, nil
}

func normalizeStepText(value, field string) (string, error) {
	if !utf8.ValidString(value) {
		return "", invalid(field)
	}
	value = stepLineBreaks.Replace(value)
	for _, char := range value {
		if unicode.IsControl(char) && char != '\n' && char != '\t' {
			return "", invalid(field)
		}
	}
	value = strings.TrimSpace(value)
	if value == "" || utf8.RuneCountInString(value) > MaxStepTextLength {
		return "", invalid(field)
	}
	return value, nil
}


func normalizeSaveRequest(req SaveRequest) (SaveRequest, error) {
	var result SaveRequest
	title, err := normalizeText(req.Title, "title", MaxTitleLength, true)
	if err != nil {
		return result, err
	}
	if req.CookingTimeMinutes < MinCookingTimeMinutes ||
		req.CookingTimeMinutes > MaxCookingTimeMinutes ||
		req.CookingTimeMinutes%CookingTimeStepMinutes != 0 {
		return result, invalid("cookingTimeMinutes")
	}
	if !validCategory(req.CategoryCode) {
		return result, invalid("categoryCode")
	}
	if len(req.Ingredients) == 0 || len(req.Ingredients) > MaxIngredients {
		return result, invalid("ingredients")
	}
	if len(req.Steps) == 0 || len(req.Steps) > MaxSteps {
		return result, invalid("steps")
	}
	result = SaveRequest{
		Title:              title,
		CookingTimeMinutes: req.CookingTimeMinutes,
		CategoryCode:       req.CategoryCode,
		Ingredients:        make([]IngredientRequest, len(req.Ingredients)),
		Steps:              make([]StepRequest, len(req.Steps)),
	}
	for i, ingredient := range req.Ingredients {
		prefix := "ingredients." + strconv.Itoa(i) + "."
		name, err := normalizeText(ingredient.Name, prefix+"name", MaxIngredientNameLength, true)
		if err != nil {
			return SaveRequest{}, err
		}
		quantity, err := normalizeQuantity(ingredient.Quantity, prefix+"quantity")
		if err != nil {
			return SaveRequest{}, err
		}
		unit, err := normalizeText(ingredient.Unit, prefix+"unit", MaxUnitLength, false)
		if err != nil {
			return SaveRequest{}, err
		}
		result.Ingredients[i] = IngredientRequest{Name: name, Quantity: quantity, Unit: unit}
	}
	totalTextLength := 0
	for i, step := range req.Steps {
		text, err := normalizeStepText(step.Text, "steps."+strconv.Itoa(i)+".text")
		if err != nil {
			return SaveRequest{}, err
		}
		totalTextLength += utf8.RuneCountInString(text)
		if totalTextLength > MaxTotalStepTextLength {
			return SaveRequest{}, invalid("steps")
		}
		result.Steps[i] = StepRequest{Text: text}
	}
	return result, nil
}
