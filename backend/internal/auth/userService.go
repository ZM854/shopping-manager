package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrInvalidCredentials = errors.New("invalid email or password")
	ErrUserNotActivated   = errors.New("user is not activated")
	ErrInvalidActivation  = errors.New("invalid or expired activation token")
	ErrActivationDelivery = errors.New("activation email delivery failed")
)

type ActivationCooldownError struct{ RetryAfter int }

func (e *ActivationCooldownError) Error() string { return "activation resend cooldown" }

type activationTransaction interface {
	LockActivation(context.Context, string) (ActivationAccount, error)
	SaveActivation(context.Context, int64, ActivationState) error
}

type userStore interface {
	Create(context.Context, CreateUserRequest) (User, error)
	GetByEmail(context.Context, string) (User, error)
	GetById(context.Context, int64) (User, error)
	Activate(context.Context, string) error
	InActivationTx(context.Context, func(activationTransaction) error) error
}

type userTokens interface {
	GenerateTokens(int64) (TokenPair, error)
	SaveToken(context.Context, int64, string) error
	ValidateRefreshToken(string) (*TokenClaims, error)
	FindToken(context.Context, int64, string) (*RefreshToken, error)
	RemoveToken(context.Context, int64) error
}

type activationMailer interface {
	SendActivationMail(context.Context, string, string) error
}

type UserService struct {
	log               *slog.Logger
	userRepository    userStore
	tokenService      userTokens
	mailService       activationMailer
	activationBaseURL string
	activationOptions ActivationOptions
	now               func() time.Time
}

func NewUserService(
	log *slog.Logger,
	userRepository userStore,
	tokenService userTokens,
	mailService activationMailer,
	activationBaseURL string,
	options ActivationOptions,
) *UserService {
	return &UserService{
		log:               log.With("component", "service", "entity", "user"),
		userRepository:    userRepository,
		tokenService:      tokenService,
		mailService:       mailService,
		activationBaseURL: activationBaseURL,
		activationOptions: options,
		now:               time.Now,
	}
}

func activationHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func (s *UserService) newActivation(now time.Time) (string, ActivationState) {
	token := uuid.NewString()
	return token, ActivationState{
		TokenHash:  activationHash(token),
		ExpiresAt:  now.Add(s.activationOptions.TTL),
		NextSendAt: now.Add(s.activationOptions.ResendInterval),
	}
}

func (s *UserService) sendActivation(ctx context.Context, email string, token string) error {
	mailCtx, cancel := context.WithTimeout(ctx, s.activationOptions.MailTimeout)
	defer cancel()
	if err := s.mailService.SendActivationMail(mailCtx, email, s.activationBaseURL+"/"+token); err != nil {
		s.log.Error("activation email delivery failed")
		return ErrActivationDelivery
	}
	return nil
}

func (s *UserService) Registration(
	ctx context.Context,
	name string,
	email string,
	password string,
) (RegistrationResponse, error) {
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return RegistrationResponse{}, err
	}
	token, state := s.newActivation(s.now())
	_, err = s.userRepository.Create(ctx, CreateUserRequest{
		Name: name, Email: email, PasswordHash: string(passwordHash), Activation: state,
	})
	if err != nil {
		return RegistrationResponse{}, err
	}
	if err := s.sendActivation(ctx, email, token); err != nil {
		return RegistrationResponse{}, err
	}
	return RegistrationResponse{ActivationRequired: true}, nil
}

func (s *UserService) Activate(ctx context.Context, token string) error {
	id, err := uuid.Parse(token)
	if err != nil || id.Version() != 4 || id.String() != token {
		return ErrInvalidActivation
	}
	return s.userRepository.Activate(ctx, activationHash(token))
}

func (s *UserService) ResendActivation(ctx context.Context, email string) error {
	var token string
	err := s.userRepository.InActivationTx(ctx, func(tx activationTransaction) error {
		account, err := tx.LockActivation(ctx, email)
		if errors.Is(err, ErrUserNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		if account.IsEmailVerified {
			return nil
		}
		now := s.now()
		if remaining := account.NextSendAt.Sub(now); remaining > 0 {
			return &ActivationCooldownError{RetryAfter: int((remaining + time.Second - 1) / time.Second)}
		}
		var state ActivationState
		token, state = s.newActivation(now)
		return tx.SaveActivation(ctx, account.ID, state)
	})
	if err != nil || token == "" {
		return err
	}
	return s.sendActivation(ctx, email, token)
}

func (s *UserService) Login(ctx context.Context, email string, password string) (AuthResult, error) {

	user, err := s.userRepository.GetByEmail(ctx, email)

	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			return AuthResult{}, ErrInvalidCredentials
		}
		return AuthResult{}, err
	}

	if err = bcrypt.CompareHashAndPassword(
		[]byte(user.PasswordHash),
		[]byte(password),
	); err != nil {
		return AuthResult{}, ErrInvalidCredentials
	}

	if user.IsEmailVerified == false {
		return AuthResult{}, ErrUserNotActivated
	}

	tokens, err := s.tokenService.GenerateTokens(user.ID)

	if err != nil {
		return AuthResult{}, err
	}

	if err = s.tokenService.SaveToken(
		ctx,
		user.ID,
		tokens.RefreshToken,
	); err != nil {
		return AuthResult{}, err
	}

	return AuthResult{
		User:   NewUserDTO(user),
		Tokens: tokens,
	}, nil
}

func (s *UserService) Logout(ctx context.Context, refreshToken string) error {
	claims, err := s.tokenService.ValidateRefreshToken(refreshToken)

	if err != nil {
		return err
	}

	return s.tokenService.RemoveToken(ctx, claims.UserID)
}

func (s *UserService) Refresh(ctx context.Context, refreshToken string) (AuthResult, error) {
	claims, err := s.tokenService.ValidateRefreshToken(refreshToken)

	if err != nil {
		return AuthResult{}, err
	}

	_, err = s.tokenService.FindToken(ctx, claims.UserID, refreshToken)

	if err != nil {
		return AuthResult{}, err
	}

	user, err := s.userRepository.GetById(ctx, claims.UserID)

	if err != nil {
		return AuthResult{}, err
	}

	if !user.IsEmailVerified {
		if err := s.tokenService.RemoveToken(ctx, user.ID); err != nil &&
			!errors.Is(err, ErrRefreshTokenNotFound) {
			return AuthResult{}, err
		}
		return AuthResult{}, ErrUserNotActivated
	}

	tokens, err := s.tokenService.GenerateTokens(user.ID)

	if err != nil {
		return AuthResult{}, err
	}

	err = s.tokenService.SaveToken(ctx, user.ID, tokens.RefreshToken)

	if err != nil {
		return AuthResult{}, err
	}

	return AuthResult{
		User:   NewUserDTO(user),
		Tokens: tokens,
	}, nil
}
