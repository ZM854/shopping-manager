package auth

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrInvalidCredentials = errors.New("invalid email or password")
	ErrUserNotActivated   = errors.New("user is not activated")
	ErrInvalidActivation  = errors.New("invalid activation token")
)

type userStore interface {
	Create(context.Context, CreateUserRequest) (User, error)
	GetByEmail(context.Context, string) (User, error)
	GetById(context.Context, int64) (User, error)
	GetByActivationToken(context.Context, string) (User, error)
	Update(context.Context, int64, UpdateUserRequest) (User, error)
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
	activationBaseUrl string
}

func NewUserService(
	log *slog.Logger,
	userRepository userStore,
	tokenService userTokens,
	mailService activationMailer,
	activationBaseUrl string,
) *UserService {
	return &UserService{
		log:               log.With("component", "service", "entity", "user"),
		userRepository:    userRepository,
		tokenService:      tokenService,
		mailService:       mailService,
		activationBaseUrl: activationBaseUrl,
	}
}

func (s *UserService) Registration(
	ctx context.Context,
	name string,
	email string,
	password string,
) (RegistrationResponse, error) {
	passwordHash, err := bcrypt.GenerateFromPassword(
		[]byte(password),
		bcrypt.DefaultCost,
	)

	if err != nil {
		s.log.Error("failed to hash password", "error", err)
		return RegistrationResponse{}, err
	}

	activationToken := uuid.NewString()

	_, err = s.userRepository.Create(
		ctx,
		CreateUserRequest{
			Name:            name,
			Email:           email,
			PasswordHash:    string(passwordHash),
			ActivationToken: activationToken,
		},
	)

	if err != nil {
		return RegistrationResponse{}, err
	}

	s.mailService.SendActivationMail(
		ctx,
		email,
		fmt.Sprintf(
			"%s/%s",
			s.activationBaseUrl,
			activationToken,
		),
	)

	return RegistrationResponse{ActivationRequired: true}, nil
}

func (s *UserService) Activate(ctx context.Context, activationLink string) error {
	user, err := s.userRepository.GetByActivationToken(ctx, activationLink)

	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			return ErrInvalidActivation
		}
		return err
	}
	_, err = s.userRepository.Update(
		ctx,
		user.ID,
		UpdateUserRequest{
			Name:            user.Name,
			Email:           user.Email,
			PasswordHash:    user.PasswordHash,
			IsEmailVerified: true,
			ActivationToken: "",
		},
	)

	return err
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
