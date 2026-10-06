package shopping

import (
	"encoding/json"
	"time"
)

const (
	MaxListNameLength = 100
	MaxItemNameLength = 200
	MaxUnitLength     = 32
	MaxQuantity       = "999999999.999"
	MaxQuantityScale  = 3
	MaxJSONBodyBytes  = 64 * 1024
)

type List struct {
	ID        int64     `json:"id,string"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type Item struct {
	ID       int64   `json:"id,string"`
	ListID   int64   `json:"listId,string"`
	Name     string  `json:"name"`
	Quantity *string `json:"quantity"`
	Unit     string  `json:"unit"`
	IsMarked bool    `json:"isMarked"`
}

type CreateListRequest struct {
	Name string `json:"name"`
}

type UpdateListRequest struct {
	Name PatchField[string] `json:"name,omitzero"`
}

type CreateItemRequest struct {
	Name     string  `json:"name"`
	Quantity *string `json:"quantity"`
	Unit     string  `json:"unit"`
	IsMarked bool    `json:"isMarked"`
}

// PatchField различает отсутствующее поле, явный null и значение.
// Допустимость null проверит service: null разрешён только для quantity.
type PatchField[T any] struct {
	Present bool
	Value   *T
}

func (f *PatchField[T]) UnmarshalJSON(data []byte) error {
	var value *T
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	f.Present, f.Value = true, value
	return nil
}

func (f PatchField[T]) IsZero() bool { return !f.Present }

func (f PatchField[T]) MarshalJSON() ([]byte, error) { return json.Marshal(f.Value) }

type UpdateItemRequest struct {
	Name     PatchField[string] `json:"name,omitzero"`
	Quantity PatchField[string] `json:"quantity,omitzero"`
	Unit     PatchField[string] `json:"unit,omitzero"`
	IsMarked PatchField[bool]   `json:"isMarked,omitzero"`
}

type MarkAllRequest struct {
	IsMarked PatchField[bool] `json:"isMarked,omitzero"`
}

type ErrorResponse struct {
	Code        string            `json:"code"`
	Message     string            `json:"message"`
	FieldErrors map[string]string `json:"fieldErrors,omitempty"`
}
