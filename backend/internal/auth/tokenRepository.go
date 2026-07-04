package auth

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type TokenRepository struct {
	db *pgxpool.Pool
	log *slog.Logger
}

var ErrRefreshTokenNotFound = errors.New("refresh token not found")

func NewTokenRepository(db *pgxpool.Pool, log *slog.Logger) *TokenRepository  {
	return &TokenRepository{
		db: db,
		log: log.With("component", "repository", "entity", "refresh_token"),
	}
}

func (r *TokenRepository) Save(ctx context.Context, userId int64, tokenHash string) error  {
	const query = `
		INSERT INTO refresh_tokens (
			user_id,
			token_hash
		)
		VALUES ($1, $2)
		ON CONFLICT (user_id)
		DO UPDATE SET
			token_hash = EXCLUDED.token_hash
		RETURNING id
	`
	start := time.Now()


	_, err := r.db.Exec(ctx, query, userId, tokenHash)

	if err != nil {
		r.log.Error("failed to save token", "user_id", userId, "error", err)
		return err
	}

	r.log.Debug("refresh token saved", "user_id", userId, "duration", time.Since(start))

	return nil
}

func (r *TokenRepository) FindByUserId(ctx context.Context, userId int64) (RefreshToken, error) {
	const query = `
		SELECT id, user_id, token_hash
		FROM refresh_tokens
		WHERE user_id = $1
	`

	start := time.Now()

	var token RefreshToken

	err := r.db.QueryRow(ctx, query, userId).Scan(
		&token.ID,
		&token.UserID,
		&token.TokenHash,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return RefreshToken{}, ErrRefreshTokenNotFound
	}

	if err != nil {
		r.log.Error("failed to find refresh token", "error", err)
		return RefreshToken{}, err
	}
	
	r.log.Debug("refresh token found", "user_id", token.UserID, "duration", time.Since(start))

	return token, nil
}

func (r *TokenRepository) DeleteByUserId(ctx context.Context, userId int64) (error) {
	const query = `
		DELETE FROM refresh_tokens
		WHERE user_id = $1
	`
	start := time.Now()

	tag, err := r.db.Exec(ctx, query, userId)

	if err != nil {
		r.log.Error("failed to delete refresh token", "user_id", userId, "error", err)
		return err
	}

	if tag.RowsAffected() == 0 {
		return ErrRefreshTokenNotFound
	}
	
	r.log.Debug("refresh token deleted", "user_id", userId, "duration", time.Since(start))

	return nil
}