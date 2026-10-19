package auth

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrUserNotFound = errors.New("user not found")
var ErrUserAlreadyExist = errors.New("user already exists")

type userQueryer interface {
	QueryRow(context.Context, string, ...any) pgx.Row
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}

type UserRepository struct {
	db   userQueryer
	pool *pgxpool.Pool
}

func NewUserRepository(db *pgxpool.Pool) *UserRepository {
	return &UserRepository{db: db, pool: db}
}

func scanUser(row pgx.Row) (User, error) {
	var user User
	err := row.Scan(&user.ID, &user.Name, &user.Email, &user.PasswordHash, &user.IsEmailVerified)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrUserNotFound
	}
	return user, err
}

func (r *UserRepository) GetByEmail(ctx context.Context, email string) (User, error) {
	return scanUser(r.db.QueryRow(ctx, `
		SELECT id, name, email, password_hash, is_email_verified
		FROM users WHERE email = $1
	`, email))
}

func (r *UserRepository) GetById(ctx context.Context, id int64) (User, error) {
	return scanUser(r.db.QueryRow(ctx, `
		SELECT id, name, email, password_hash, is_email_verified
		FROM users WHERE id = $1
	`, id))
}

func (r *UserRepository) Create(ctx context.Context, req CreateUserRequest) (User, error) {
	user, err := scanUser(r.db.QueryRow(ctx, `
		INSERT INTO users (
			name, email, password_hash, activation_token_hash,
			activation_expires_at, activation_next_send_at
		) VALUES ($1, $2, $3, NULLIF($4, ''), $5, $6)
		RETURNING id, name, email, password_hash, is_email_verified
	`, req.Name, req.Email, req.PasswordHash, req.Activation.TokenHash,
		nullableTime(req.Activation.ExpiresAt), nullableTime(req.Activation.NextSendAt)))
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "users_email_key" {
		return User{}, ErrUserAlreadyExist
	}
	return user, err
}

func (r *UserRepository) Activate(ctx context.Context, hash string) error {
	tag, err := r.db.Exec(ctx, `
		UPDATE users SET
			is_email_verified = true,
			activation_token_hash = NULL,
			activation_expires_at = NULL,
			activation_next_send_at = NULL
		WHERE activation_token_hash = $1
			AND NOT is_email_verified
			AND activation_expires_at > clock_timestamp()
	`, hash)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrInvalidActivation
	}
	return nil
}

func (r *UserRepository) InActivationTx(ctx context.Context, fn func(activationTransaction) error) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	if err := fn(&UserRepository{db: tx}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *UserRepository) LockActivation(ctx context.Context, email string) (ActivationAccount, error) {
	var account ActivationAccount
	var next *time.Time
	err := r.db.QueryRow(ctx, `
		SELECT id, is_email_verified, activation_next_send_at
		FROM users WHERE email = $1 FOR UPDATE
	`, email).Scan(&account.ID, &account.IsEmailVerified, &next)
	if errors.Is(err, pgx.ErrNoRows) {
		return ActivationAccount{}, ErrUserNotFound
	}
	if next != nil {
		account.NextSendAt = *next
	}
	return account, err
}

func (r *UserRepository) SaveActivation(ctx context.Context, id int64, state ActivationState) error {
	_, err := r.db.Exec(ctx, `
		UPDATE users SET
			activation_token_hash = $2,
			activation_expires_at = $3,
			activation_next_send_at = $4
		WHERE id = $1
	`, id, state.TokenHash, state.ExpiresAt, state.NextSendAt)
	return err
}

func nullableTime(value time.Time) *time.Time {
	if value.IsZero() {
		return nil
	}
	return &value
}
