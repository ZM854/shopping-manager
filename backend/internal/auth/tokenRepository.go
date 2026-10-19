package auth

import (
	"context"
	"errors"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrRefreshTokenNotFound = errors.New("refresh session not found")

type TokenRepository struct {
	pool *pgxpool.Pool
}

func NewTokenRepository(db *pgxpool.Pool, _ *slog.Logger) *TokenRepository {
	return &TokenRepository{pool: db}
}

func (r *TokenRepository) Create(ctx context.Context, session RefreshToken) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO refresh_tokens (session_id, user_id, token_hash, expires_at)
		VALUES ($1, $2, $3, $4)`,
		session.SessionID, session.UserID, session.TokenHash, session.ExpiresAt,
	)
	return err
}

func (r *TokenRepository) Delete(ctx context.Context, userID int64, sessionID, hash string) error {
	_, err := r.pool.Exec(ctx, `
		DELETE FROM refresh_tokens
		WHERE user_id = $1 AND session_id = $2 AND token_hash = $3`,
		userID, sessionID, hash,
	)
	return err
}

func (r *TokenRepository) InSessionTx(ctx context.Context, fn func(sessionTransaction) error) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := fn(&sessionRepository{tx: tx}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

type sessionRepository struct{ tx pgx.Tx }

func (r *sessionRepository) Lock(ctx context.Context, userID int64, sessionID string) (RefreshToken, User, error) {
	var session RefreshToken
	var user User
	err := r.tx.QueryRow(ctx, `
		SELECT rt.session_id::text, rt.user_id, rt.token_hash,
		       rt.created_at, rt.expires_at, rt.last_used_at,
		       u.id, u.name, u.email, u.is_email_verified
		FROM refresh_tokens rt JOIN users u ON u.id = rt.user_id
		WHERE rt.user_id = $1 AND rt.session_id = $2
		  AND rt.expires_at > clock_timestamp()
		FOR UPDATE OF rt`, userID, sessionID,
	).Scan(
		&session.SessionID, &session.UserID, &session.TokenHash,
		&session.CreatedAt, &session.ExpiresAt, &session.LastUsedAt,
		&user.ID, &user.Name, &user.Email, &user.IsEmailVerified,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return RefreshToken{}, User{}, ErrRefreshTokenNotFound
	}
	return session, user, err
}

func (r *sessionRepository) Rotate(ctx context.Context, session RefreshToken, newHash string) error {
	result, err := r.tx.Exec(ctx, `
		UPDATE refresh_tokens
		SET token_hash = $4, last_used_at = clock_timestamp()
		WHERE user_id = $1 AND session_id = $2 AND token_hash = $3
		  AND expires_at > clock_timestamp()`,
		session.UserID, session.SessionID, session.TokenHash, newHash,
	)
	if err != nil {
		return err
	}
	if result.RowsAffected() != 1 {
		return ErrRefreshTokenNotFound
	}
	return nil
}

func (r *TokenRepository) CleanupExpired(ctx context.Context) (int64, error) {
	result, err := r.pool.Exec(ctx, `DELETE FROM refresh_tokens WHERE expires_at <= clock_timestamp()`)
	return result.RowsAffected(), err
}
