package recipe

import (
	"context"
	"time"
)

// Claim сериализует конкурирующие создания с одним ключом до конца транзакции.
// Истёкшую запись можно использовать заново; решение о конфликте хеша — в service.
func (r *transactionRepository) Claim(ctx context.Context, owner int64, key, hash string, expires time.Time) (Replay, error) {
	var result Replay
	err := r.db.QueryRow(ctx, `
		INSERT INTO recipe_idempotency AS old (owner_id, key, request_hash, expires_at)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (owner_id, key) DO UPDATE SET
			request_hash = CASE WHEN old.expires_at <= statement_timestamp()
				THEN EXCLUDED.request_hash ELSE old.request_hash END,
			resource_id = CASE WHEN old.expires_at <= statement_timestamp()
				THEN NULL ELSE old.resource_id END,
			response = CASE WHEN old.expires_at <= statement_timestamp()
				THEN NULL ELSE old.response END,
			expires_at = CASE WHEN old.expires_at <= statement_timestamp()
				THEN EXCLUDED.expires_at ELSE old.expires_at END
		RETURNING request_hash, resource_id, response`, owner, key, hash, expires,
	).Scan(&result.Hash, &result.ResourceID, &result.Response)
	return result, err
}

func (r *transactionRepository) SaveReplay(ctx context.Context, owner int64, key string, id int64, response []byte) error {
	return changed(r.db.Exec(ctx, `
		UPDATE recipe_idempotency SET resource_id = $3, response = $4::jsonb,
			expires_at = greatest(expires_at, clock_timestamp() + interval '24 hours')
		WHERE owner_id = $1 AND key = $2`, owner, key, id, response,
	))
}
