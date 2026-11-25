package recipe

import (
	"time"

	"github.com/ZM854/shopping-manager/backend/internal/media"
)

const (
	MaxTitleLength          = 200
	MaxIngredientNameLength = 200
	MaxUnitLength           = 32
	MinCookingTimeMinutes   = 5
	MaxCookingTimeMinutes   = 1440
	CookingTimeStepMinutes  = 5
	MaxIngredients          = 200
	MaxSteps                = 100
	MaxStepTextLength       = 20000
	MaxTotalStepTextLength  = 50000
	MaxQuantity             = "999999999.999"
	MaxQuantityScale        = 3
	MaxJSONBodyBytes        = 1024 * 1024
)

type Summary struct {
	ID                 int64           `json:"id,string"`
	Title              string          `json:"title"`
	CookingTimeMinutes int             `json:"cookingTimeMinutes"`
	CategoryCode       string          `json:"categoryCode"`
	CreatedAt          time.Time       `json:"createdAt"`
	UpdatedAt          time.Time       `json:"updatedAt"`
	Cover              *media.MediaDTO `json:"cover"`
}

type Recipe struct {
	Summary
	Ingredients []Ingredient `json:"ingredients"`
	Steps       []Step       `json:"steps"`
}

type Ingredient struct {
	Position int     `json:"position"`
	Name     string  `json:"name"`
	Quantity *string `json:"quantity"`
	Unit     string  `json:"unit"`
}

type Step struct {
	ID       int64            `json:"id,string"`
	Position int              `json:"position"`
	Text     string           `json:"text"`
	Media    []media.MediaDTO `json:"media"`
}

type SaveRequest struct {
	Title              string              `json:"title"`
	CookingTimeMinutes int                 `json:"cookingTimeMinutes"`
	CategoryCode       string              `json:"categoryCode"`
	Ingredients        []IngredientRequest `json:"ingredients"`
	Steps              []StepRequest       `json:"steps"`
	CoverMediaID       *string             `json:"coverMediaId"`
}

type IngredientRequest struct {
	Name     string  `json:"name"`
	Quantity *string `json:"quantity"`
	Unit     string  `json:"unit"`
}

type StepRequest struct {
	ID       *string  `json:"id,omitempty"`
	Text     string   `json:"text"`
	MediaIDs []string `json:"mediaIds"`
}

type ErrorResponse struct {
	Code        string            `json:"code"`
	Message     string            `json:"message"`
	FieldErrors map[string]string `json:"fieldErrors,omitempty"`
}
