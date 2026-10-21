package recipe

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type queryer interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

type Repository struct {
	db   queryer
	pool *pgxpool.Pool
}

type transactionRepository struct {
	*Repository
}

var _ Store = (*Repository)(nil)
var _ Transaction = (*transactionRepository)(nil)

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{db: pool, pool: pool}
}

func (r *Repository) InTx(ctx context.Context, fn func(Transaction) error) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	if err := fn(&transactionRepository{Repository: &Repository{db: tx}}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func missing(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

func changed(tag pgconn.CommandTag, err error) error {
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func scanSummary(row pgx.Row) (Summary, error) {
	var result Summary
	if err := row.Scan(
		&result.ID, &result.Title, &result.CookingTimeMinutes,
		&result.CategoryCode, &result.CreatedAt, &result.UpdatedAt,
	); err != nil {
		return Summary{}, missing(err)
	}
	result.CreatedAt = result.CreatedAt.UTC()
	result.UpdatedAt = result.UpdatedAt.UTC()
	return result, nil
}

func (r *Repository) GetRecipes(ctx context.Context, owner int64) ([]Summary, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, title, cooking_time_minutes, category_code, created_at, updated_at
		FROM recipes WHERE owner_id = $1 ORDER BY id`, owner)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]Summary, 0)
	for rows.Next() {
		item, err := scanSummary(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (r *Repository) GetRecipe(ctx context.Context, owner, id int64) (Recipe, error) {
	var result Recipe
	var ingredients, steps []byte

	err := r.db.QueryRow(ctx, `
		SELECT r.id, r.title, r.cooking_time_minutes, r.category_code,
			r.created_at, r.updated_at,
			COALESCE((
				SELECT jsonb_agg(jsonb_build_object(
					'position', i.position, 'name', i.name,
					'quantity', trim_scale(i.quantity)::text, 'unit', i.unit
				) ORDER BY i.position)
				FROM recipe_ingredients i WHERE i.recipe_id = r.id
			), '[]'::jsonb),
			COALESCE((
				SELECT jsonb_agg(jsonb_build_object(
					'position', s.position, 'text', s.text
				) ORDER BY s.position)
				FROM recipe_steps s WHERE s.recipe_id = r.id
			), '[]'::jsonb)
		FROM recipes r WHERE r.owner_id = $1 AND r.id = $2`, owner, id,
	).Scan(
		&result.ID, &result.Title, &result.CookingTimeMinutes,
		&result.CategoryCode, &result.CreatedAt, &result.UpdatedAt,
		&ingredients, &steps,
	)
	if err != nil {
		return Recipe{}, missing(err)
	}
	if err := json.Unmarshal(ingredients, &result.Ingredients); err != nil {
		return Recipe{}, err
	}
	if err := json.Unmarshal(steps, &result.Steps); err != nil {
		return Recipe{}, err
	}
	result.CreatedAt = result.CreatedAt.UTC()
	result.UpdatedAt = result.UpdatedAt.UTC()
	return result, nil
}

func (r *transactionRepository) LockRecipe(ctx context.Context, owner, id int64) error {
	var found int64
	return missing(r.db.QueryRow(ctx, `
		SELECT id FROM recipes WHERE owner_id = $1 AND id = $2 FOR UPDATE`,
		owner, id,
	).Scan(&found))
}

func (r *transactionRepository) CreateRecipe(ctx context.Context, owner int64, req SaveRequest) (Recipe, error) {
	var id int64
	err := r.db.QueryRow(ctx, `
		INSERT INTO recipes (owner_id, title, cooking_time_minutes, category_code)
		VALUES ($1, $2, $3, $4) RETURNING id`,
		owner, req.Title, req.CookingTimeMinutes, req.CategoryCode,
	).Scan(&id)
	if err != nil {
		return Recipe{}, err
	}
	if err := r.replaceContents(ctx, owner, id, req); err != nil {
		return Recipe{}, err
	}
	return r.GetRecipe(ctx, owner, id)
}

func (r *transactionRepository) UpdateRecipe(ctx context.Context, owner, id int64, req SaveRequest) (Recipe, error) {
	err := changed(r.db.Exec(ctx, `
		UPDATE recipes SET title = $3, cooking_time_minutes = $4, category_code = $5,
			updated_at = greatest(updated_at, clock_timestamp())
		WHERE owner_id = $1 AND id = $2`,
		owner, id, req.Title, req.CookingTimeMinutes, req.CategoryCode,
	))
	if err != nil {
		return Recipe{}, err
	}
	if err := r.replaceContents(ctx, owner, id, req); err != nil {
		return Recipe{}, err
	}
	return r.GetRecipe(ctx, owner, id)
}


func (r *transactionRepository) replaceContents(ctx context.Context, owner, id int64, req SaveRequest) error {
	if _, err := r.db.Exec(ctx, `
		DELETE FROM recipe_ingredients i USING recipes r
		WHERE i.recipe_id = r.id AND r.owner_id = $1 AND r.id = $2`, owner, id,
	); err != nil {
		return err
	}
	if _, err := r.db.Exec(ctx, `
		DELETE FROM recipe_steps s USING recipes r
		WHERE s.recipe_id = r.id AND r.owner_id = $1 AND r.id = $2`, owner, id,
	); err != nil {
		return err
	}

	names := make([]string, len(req.Ingredients))
	quantities := make([]*string, len(req.Ingredients))
	units := make([]string, len(req.Ingredients))
	for i, ingredient := range req.Ingredients {
		names[i], quantities[i], units[i] = ingredient.Name, ingredient.Quantity, ingredient.Unit
	}
	if _, err := r.db.Exec(ctx, `
		INSERT INTO recipe_ingredients (recipe_id, position, name, quantity, unit)
		SELECT r.id, i.position, i.name, i.quantity::numeric, i.unit
		FROM recipes r CROSS JOIN
			unnest($3::text[], $4::text[], $5::text[]) WITH ORDINALITY
			AS i(name, quantity, unit, position)
		WHERE r.owner_id = $1 AND r.id = $2`, owner, id, names, quantities, units,
	); err != nil {
		return err
	}
	texts := make([]string, len(req.Steps))
	for i, step := range req.Steps {
		texts[i] = step.Text
	}
	_, err := r.db.Exec(ctx, `
		INSERT INTO recipe_steps (recipe_id, position, text)
		SELECT r.id, s.position, s.text
		FROM recipes r CROSS JOIN unnest($3::text[]) WITH ORDINALITY AS s(text, position)
		WHERE r.owner_id = $1 AND r.id = $2`, owner, id, texts)
	return err
}

func (r *transactionRepository) DeleteRecipe(ctx context.Context, owner, id int64) error {
	return changed(r.db.Exec(ctx, `
		DELETE FROM recipes WHERE owner_id = $1 AND id = $2`, owner, id,
	))
}
