package auth

import (
	"errors"
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrUserNotFound = errors.New("User not found")


type UserRepository struct {
	db *pgxpool.Pool
	log *slog.Logger
}

func (r *UserRepository) GetByEmail(ctx *gin.Context, email string) (User, error) {
	const query = `
		SELECT id, email, password_hash, is_email_verified, activation_token
		FROM users
		WHERE email = $1
	`

	start := time.Now()

	var user User

	err := r.db.QueryRow(ctx, query, email).Scan(
		&user.ID,
		&user.Email,
		&user.PasswordHash,
		&user.IsEmailVerified,
		&user.ActivationToken,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrUserNotFound
	}

	if err != nil {
		r.log.Error(
			"failed to get user by email",
			"user_email", email,
			"error", err,
		)
		return User{}, err
	}

	r.log.Debug(
		"get user by email completed",
		"user_id", user.ID,
		"user_email", email,
		"duration", time.Since(start),
	)

	return user, nil
}

func (r *UserRepository) GetById(ctx *gin.Context, id int64) (User, error) {
	const query = `
		SELECT id, email, password_hash, is_email_verified, activation_token
		FROM users
		WHERE id = $1
	`

	start := time.Now()

	var user User

	err := r.db.QueryRow(ctx, query, id).Scan(
		&user.ID,
		&user.Email,
		&user.PasswordHash,
		&user.IsEmailVerified,
		&user.ActivationToken,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrUserNotFound
	}

	if err != nil {
		r.log.Error(
			"failed to get user by email",
			"user_id", id,
			"error", err,
		)
		return User{}, err
	}

	r.log.Debug(
		"get user by email completed",
		"user_id", user.ID,
		"duration", time.Since(start),
	)

	return user, nil
}

func (r *UserRepository) Create(ctx *gin.Context,  userData CreateUserRequest) (User, error)  {
	const query = `
		INSERT INTO users (
			email,
			password_hash,
			activation_token
		)
		VALUES ($1, $2, $3)
		RETURNING id
	`

	start := time.Now()

	newUser := User{
		Email: userData.Email,
		PasswordHash: userData.PasswordHash,
		IsEmailVerified: false,
		ActivationToken: userData.ActivationToken,
	}

	err := r.db.QueryRow(
		ctx, query, 
		userData.Email, 
		userData.PasswordHash, 
		userData.ActivationToken,
	).Scan(&newUser.ID)

	if err != nil {
		r.log.Error(
			"failed to create user", 
			"user_email", userData.Email, 
			"error", err,
		)
		return User{}, err
	}

	r.log.Debug(
		"user created", 
		"user_id", newUser.ID,
		"duration", time.Since(start),
	)

	return newUser, nil	
}