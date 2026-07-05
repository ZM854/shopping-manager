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

type UserService struct {
	log *slog.Logger
	userRepository *UserRepository
	tokenService *TokenService
	mailService *MailService
	activationBaseUrl string
}

func NewUserService(
	log *slog.Logger, 
	userRepository *UserRepository,
	tokenService *TokenService,
	mailService *MailService,
	activationBaseUrl string,
) *UserService {
	return &UserService{
		log: log.With("component", "service", "entity", "user"),
		userRepository: userRepository,
		tokenService: tokenService,
		mailService: mailService,
		activationBaseUrl: activationBaseUrl,
	}
}

func (s *UserService) Registration(
	ctx context.Context,
	email string,
	password string,

) (AuthResponse, error)  {
	passwordHash, err := bcrypt.GenerateFromPassword(
		[]byte(password),
		bcrypt.DefaultCost,
	)
	if err != nil {
		s.log.Error("failed to hash password", "error", err)
		return AuthResponse{}, err
	}
	activationToken := uuid.NewString()

	user, err := s.userRepository.Create(
		ctx,
		CreateUserRequest{
			Email: email,
			PasswordHash: string(passwordHash),
			ActivationToken: activationToken,
		},
	)

	if errors.Is(err, ErrUserAlreadyExist) {
		return AuthResponse{}, err
	}

	if err != nil {
		return AuthResponse{}, err
	}

	s.mailService.SendActivationMail(
		email,
		fmt.Sprintf(
			"%s/%s", 
			s.activationBaseUrl, 
			activationToken,
		),
	)
	tokens, err := s.tokenService.GenerateTokens(user.ID)

	if err != nil {
		return AuthResponse{}, err
	}

	if err := s.tokenService.SaveToken(
		ctx, user.ID,
		tokens.RefreshToken,
	); err != nil {
		return AuthResponse{}, err
	}

	return AuthResponse{
		User: NewUserDTO(user),
		TokenPair: tokens,
	}, nil
}

func (s *UserService) Activate(ctx context.Context, activationLink string) error {
	user, err := s.userRepository.GetByActivationToken(ctx, activationLink)

	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			return  ErrInvalidActivation
		}
		return err
	}
	_, err = s.userRepository.Update(
		ctx,
		user.ID,
		UpdateUserRequest{
			Email: user.Email,
			PasswordHash: user.PasswordHash,
			IsEmailVerified: true,
			ActivationToken: "",
		},
	)

	return err
}

func (s *UserService) Login(ctx context.Context, email string, password string) (AuthResponse, error)  {

	user, err := s.userRepository.GetByEmail(ctx, email)

	if err != nil {

		if errors.Is(err, ErrUserNotFound) {
			return AuthResponse{}, ErrInvalidCredentials
		}
		return AuthResponse{}, err
	}

	if err = bcrypt.CompareHashAndPassword(
		[]byte(user.PasswordHash),
		[]byte(password),
	); err != nil {
		return AuthResponse{}, ErrInvalidCredentials
	}

	tokens, err := s.tokenService.GenerateTokens(user.ID)

	if err != nil {
		return AuthResponse{}, err
	}

    if err = s.tokenService.SaveToken(
		ctx, 
		user.ID, 
		tokens.RefreshToken,
	); err != nil {
		return AuthResponse{}, err
	}

	return AuthResponse{
		User: NewUserDTO(user),
		TokenPair: tokens,
	}, err
}

func (s *UserService) Logout(ctx context.Context, refreshToken string) error {
	claims, err := s.tokenService.ValidateRefreshToken(refreshToken)

	if err != nil {
		return err
	}

	return s.tokenService.RemoveToken(ctx, claims.UserID)
}

func (s *UserService) Refresh(ctx context.Context, refreshToken string) (AuthResponse, error)  {
	claims, err := s.tokenService.ValidateRefreshToken(refreshToken)

	if err != nil {
		return AuthResponse{}, err
	}

	_, err = s.tokenService.FindToken(ctx, claims.UserID, refreshToken)

	if err != nil {
		return AuthResponse{}, err
	}

	user, err := s.userRepository.GetById(ctx, claims.UserID)

	if err != nil {
		return AuthResponse{}, err
	}

	tokens, err := s.tokenService.GenerateTokens(user.ID)

	if err != nil {
		return AuthResponse{}, err
	}

	err = s.tokenService.SaveToken(ctx, user.ID, tokens.RefreshToken)

	if err != nil {
		return AuthResponse{}, err
	}

	return AuthResponse{
		User: NewUserDTO(user),
		TokenPair: tokens,
	}, nil
}

func (s *UserService) GetAllUsers(
	ctx context.Context,
) ([]User, error) {

	return s.userRepository.GetAll(ctx)
}