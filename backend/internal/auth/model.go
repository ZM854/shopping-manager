package auth

import (
	"strconv"
	"time"
)

type User struct {
	ID              int64
	Name            string
	Email           string
	PasswordHash    string `json:"-"`
	IsEmailVerified bool
}

type CreateUserRequest struct {
	Name         string
	Email        string
	PasswordHash string          `json:"-"`
	Activation   ActivationState `json:"-"`
}

type ActivationState struct {
	TokenHash  string `json:"-"`
	ExpiresAt  time.Time
	NextSendAt time.Time
}

type ActivationAccount struct {
	ID              int64
	IsEmailVerified bool
	NextSendAt      time.Time
}

type ActivationOptions struct {
	TTL            time.Duration
	ResendInterval time.Duration
	MailTimeout    time.Duration
}

func DefaultActivationOptions() ActivationOptions {
	return ActivationOptions{TTL: 24 * time.Hour, ResendInterval: time.Minute, MailTimeout: 10 * time.Second}
}

type RefreshToken struct {
	SessionID  string `json:"-"`
	UserID     int64
	TokenHash  string `json:"-"`
	CreatedAt  time.Time
	ExpiresAt  time.Time
	LastUsedAt time.Time
}

type TokenPair struct {
	AccessToken      string    `json:"-"`
	RefreshToken     string    `json:"-"`
	SessionID        string    `json:"-"`
	RefreshExpiresAt time.Time `json:"-"`
}

type UserDTO struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	Email           string `json:"email"`
	IsEmailVerified bool   `json:"isEmailVerified"`
}

func NewUserDTO(user User) UserDTO {
	return UserDTO{
		ID:              strconv.FormatInt(user.ID, 10),
		Name:            user.Name,
		Email:           user.Email,
		IsEmailVerified: user.IsEmailVerified,
	}
}

type AuthResponse struct {
	User        UserDTO `json:"user"`
	AccessToken string  `json:"accessToken"`
}

type AuthResult struct {
	User   UserDTO
	Tokens TokenPair `json:"-"`
}

type RegistrationResponse struct {
	ActivationRequired bool `json:"activationRequired"`
}

type RegistrationRequest struct {
	Name     string `json:"name"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type ActivationRequest struct {
	Token string `json:"token" binding:"required,uuid4"`
}

type ResendActivationRequest struct {
	Email string `json:"email"`
}
