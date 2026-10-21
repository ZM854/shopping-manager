package shopping

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func scanShare(row pgx.Row) (Share, error) {
	var share Share
	err := row.Scan(&share.ID, &share.CreatedAt, &share.PreserveMarks)
	share.CreatedAt = share.CreatedAt.UTC()
	return share, missing(err)
}

func (r *Repository) GetShare(ctx context.Context, creator, share int64) (Share, error) {
	return scanShare(r.db.QueryRow(ctx, `
		SELECT id, created_at, preserve_marks FROM shopping_list_shares
		WHERE creator_id = $1 AND id = $2`, creator, share))
}

func (r *Repository) CreateShare(
	ctx context.Context, creator, list int64, tokenHash []byte, preserve bool,
) (Share, error) {
	share, err := scanShare(r.db.QueryRow(ctx, `
		INSERT INTO shopping_list_shares (creator_id, source_list_id, token_hash, name, preserve_marks)
		SELECT owner_id, id, $3, name, $4 FROM shopping_lists
		WHERE owner_id = $1 AND id = $2
		RETURNING id, created_at, preserve_marks`, creator, list, tokenHash, preserve))
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" &&
		pgErr.ConstraintName == "shopping_list_shares_token_hash_key" {
		return Share{}, ErrShareTokenConflict
	}
	return share, err
}

func (r *Repository) CopyListToShare(ctx context.Context, creator, list, share int64) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO shopping_list_share_items (share_id, position, name, quantity, unit, is_marked)
		SELECT s.id, row_number() OVER (ORDER BY i.id), i.name, i.quantity, i.unit,
		       s.preserve_marks AND i.is_marked
		FROM shopping_items i
		JOIN shopping_lists l ON l.id = i.list_id
		JOIN shopping_list_shares s ON s.source_list_id = l.id AND s.creator_id = l.owner_id
		WHERE l.owner_id = $1 AND l.id = $2 AND s.id = $3
		ORDER BY i.id`, creator, list, share)
	return err
}

func (r *Repository) FindShare(ctx context.Context, tokenHash []byte) (int64, string, error) {
	var id int64
	var name string
	err := r.db.QueryRow(ctx, `
		SELECT id, name FROM shopping_list_shares
		WHERE token_hash = $1 FOR KEY SHARE`, tokenHash).Scan(&id, &name)
	return id, name, missing(err)
}

func (r *Repository) CopyShareToList(ctx context.Context, recipient, list, share int64) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO shopping_items (list_id, name, quantity, unit, is_marked)
		SELECT l.id, i.name, i.quantity, i.unit, i.is_marked
		FROM shopping_list_share_items i
		JOIN shopping_lists l ON l.id = $2 AND l.owner_id = $1
		WHERE i.share_id = $3 ORDER BY i.position`, recipient, list, share)
	return err
}
