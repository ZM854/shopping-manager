package recipe

import (
	"context"
	"time"
)

type Reader interface {
	GetRecipes(context.Context, int64) ([]Summary, error)
	GetRecipe(context.Context, int64, int64) (Recipe, error)
}

type Transaction interface {
	Reader
	LockRecipe(context.Context, int64, int64) error
	CreateRecipe(context.Context, int64, SaveRequest) (Recipe, error)
	UpdateRecipe(context.Context, int64, int64, SaveRequest) (Recipe, error)
	DeleteRecipe(context.Context, int64, int64) error
	Claim(context.Context, int64, string, string, time.Time) (Replay, error)
	SaveReplay(context.Context, int64, string, int64, []byte) error
}

type Store interface {
	Reader
	InTx(context.Context, func(Transaction) error) error
}

type Replay struct {
	Hash       string
	ResourceID *int64
	Response   []byte
}
