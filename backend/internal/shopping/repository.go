package shopping

import (
	"context"
	"errors"
	"time"

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

func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{db: pool, pool: pool} }

func (r *Repository) InTx(ctx context.Context, fn func(Transaction) error) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	if err := fn(&Repository{db: tx}); err != nil {
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
func scanList(row pgx.Row) (List, error) {
	var list List
	err := row.Scan(&list.ID, &list.Name, &list.CreatedAt, &list.UpdatedAt)
	list.CreatedAt, list.UpdatedAt = list.CreatedAt.UTC(), list.UpdatedAt.UTC()
	return list, missing(err)
}
func scanItem(row pgx.Row) (Item, error) {
	var item Item
	err := row.Scan(&item.ID, &item.ListID, &item.Name, &item.Quantity, &item.Unit, &item.IsMarked)
	return item, missing(err)
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

func (r *Repository) GetLists(ctx context.Context, user int64) ([]List, error) {
	rows, err := r.db.Query(ctx, `SELECT id, name, created_at, updated_at FROM shopping_lists WHERE owner_id = $1 ORDER BY id`, user)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	lists := make([]List, 0)
	for rows.Next() {
		list, err := scanList(rows)
		if err != nil {
			return nil, err
		}
		lists = append(lists, list)
	}
	return lists, rows.Err()
}
func (r *Repository) GetList(ctx context.Context, user, list int64) (List, error) {
	return scanList(r.db.QueryRow(ctx, `SELECT id, name, created_at, updated_at FROM shopping_lists WHERE owner_id = $1 AND id = $2`, user, list))
}
func (r *Repository) LockList(ctx context.Context, user, list int64) error {
	var id int64
	return missing(r.db.QueryRow(ctx, `SELECT id FROM shopping_lists WHERE owner_id = $1 AND id = $2 FOR UPDATE`, user, list).Scan(&id))
}
func (r *Repository) CreateList(ctx context.Context, user int64, req CreateListRequest) (List, error) {
	return scanList(r.db.QueryRow(ctx, `INSERT INTO shopping_lists (owner_id, name) VALUES ($1, $2) RETURNING id, name, created_at, updated_at`, user, req.Name))
}
func (r *Repository) UpdateList(ctx context.Context, user, list int64, name string) (List, error) {
	return scanList(r.db.QueryRow(ctx, `UPDATE shopping_lists SET name = $3, updated_at = greatest(updated_at, clock_timestamp()) WHERE owner_id = $1 AND id = $2 RETURNING id, name, created_at, updated_at`, user, list, name))
}
func (r *Repository) DeleteList(ctx context.Context, user, list int64) error {
	return changed(r.db.Exec(ctx, `DELETE FROM shopping_lists WHERE owner_id = $1 AND id = $2`, user, list))
}
func (r *Repository) TouchList(ctx context.Context, user, list int64) error {
	return changed(r.db.Exec(ctx, `UPDATE shopping_lists SET updated_at = greatest(updated_at, clock_timestamp()) WHERE owner_id = $1 AND id = $2`, user, list))
}

func (r *Repository) GetItems(ctx context.Context, user, list int64) ([]Item, error) {
	rows, err := r.db.Query(ctx, `SELECT i.id, trim_scale(i.quantity)::text, i.name, i.unit, i.is_marked
        FROM shopping_lists l LEFT JOIN shopping_items i ON i.list_id = l.id
        WHERE l.owner_id = $1 AND l.id = $2 ORDER BY i.id`, user, list)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]Item, 0)
	found := false
	for rows.Next() {
		found = true
		var id *int64
		var quantity, name, unit *string
		var marked *bool
		if err := rows.Scan(&id, &quantity, &name, &unit, &marked); err != nil {
			return nil, err
		}
		if id != nil {
			items = append(items, Item{ID: *id, ListID: list, Name: *name, Quantity: quantity, Unit: *unit, IsMarked: *marked})
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if !found {
		return nil, ErrNotFound
	}
	return items, nil
}
func (r *Repository) GetItem(ctx context.Context, user, list, item int64) (Item, error) {
	return scanItem(r.db.QueryRow(ctx, `SELECT i.id, i.list_id, i.name, trim_scale(i.quantity)::text, i.unit, i.is_marked
        FROM shopping_items i JOIN shopping_lists l ON l.id = i.list_id
        WHERE l.owner_id = $1 AND l.id = $2 AND i.id = $3`, user, list, item))
}
func (r *Repository) CreateItem(ctx context.Context, user, list int64, req CreateItemRequest) (Item, error) {
	return scanItem(r.db.QueryRow(ctx, `INSERT INTO shopping_items (list_id, name, quantity, unit, is_marked)
        SELECT id, $3, $4::text::numeric, $5, $6 FROM shopping_lists WHERE owner_id = $1 AND id = $2
        RETURNING id, list_id, name, trim_scale(quantity)::text, unit, is_marked`, user, list, req.Name, req.Quantity, req.Unit, req.IsMarked))
}
func (r *Repository) UpdateItem(ctx context.Context, user, list, item int64, req UpdateItemRequest) (Item, error) {
	return scanItem(r.db.QueryRow(ctx, `UPDATE shopping_items i SET
        name = CASE WHEN $4 THEN $5::text ELSE i.name END,
        quantity = CASE WHEN $6 THEN $7::text::numeric ELSE i.quantity END,
        unit = CASE WHEN $8 THEN $9::text ELSE i.unit END,
        is_marked = CASE WHEN $10 THEN $11::boolean ELSE i.is_marked END
        FROM shopping_lists l WHERE l.id = i.list_id AND l.owner_id = $1 AND l.id = $2 AND i.id = $3
        RETURNING i.id, i.list_id, i.name, trim_scale(i.quantity)::text, i.unit, i.is_marked`,
		user, list, item, req.Name.Present, req.Name.Value, req.Quantity.Present, req.Quantity.Value,
		req.Unit.Present, req.Unit.Value, req.IsMarked.Present, req.IsMarked.Value))
}
func (r *Repository) DeleteItem(ctx context.Context, user, list, item int64) error {
	return changed(r.db.Exec(ctx, `DELETE FROM shopping_items i USING shopping_lists l
        WHERE l.id = i.list_id AND l.owner_id = $1 AND l.id = $2 AND i.id = $3`, user, list, item))
}
func (r *Repository) ClearItems(ctx context.Context, user, list int64) error {
	_, err := r.db.Exec(ctx, `DELETE FROM shopping_items i USING shopping_lists l
        WHERE l.id = i.list_id AND l.owner_id = $1 AND l.id = $2`, user, list)
	return err
}
func (r *Repository) MarkAll(ctx context.Context, user, list int64, marked bool) error {
	_, err := r.db.Exec(ctx, `UPDATE shopping_items i SET is_marked = $3 FROM shopping_lists l
        WHERE l.id = i.list_id AND l.owner_id = $1 AND l.id = $2`, user, list, marked)
	return err
}

func (r *Repository) Claim(ctx context.Context, user int64, scope, key, hash string, expires time.Time) (Replay, error) {
	var replay Replay
	err := r.db.QueryRow(ctx, `INSERT INTO shopping_idempotency AS old (owner_id, operation, key, request_hash, expires_at)
        VALUES ($1, $2, $3, $4, $5)
        ON CONFLICT (owner_id, operation, key) DO UPDATE SET
            request_hash = CASE WHEN old.expires_at <= statement_timestamp() THEN EXCLUDED.request_hash ELSE old.request_hash END,
            resource_id = CASE WHEN old.expires_at <= statement_timestamp() THEN NULL ELSE old.resource_id END,
            response = CASE WHEN old.expires_at <= statement_timestamp() THEN NULL ELSE old.response END,
            expires_at = CASE WHEN old.expires_at <= statement_timestamp() THEN EXCLUDED.expires_at ELSE old.expires_at END
        RETURNING request_hash, resource_id, response`, user, scope, key, hash, expires).Scan(&replay.Hash, &replay.ResourceID, &replay.Response)
	return replay, err
}
func (r *Repository) SaveReplay(ctx context.Context, user int64, scope, key string, id int64, response []byte) error {
	return changed(r.db.Exec(ctx, `UPDATE shopping_idempotency SET resource_id = $4, response = $5::jsonb,
        expires_at = greatest(expires_at, clock_timestamp() + interval '24 hours')
        WHERE owner_id = $1 AND operation = $2 AND key = $3`, user, scope, key, id, response))
}
