package auth

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrInvalidToken = errors.New("invalid token")
)

type TokenService struct {
	log *slog.Logger
	tokenRepository *TokenRepository

	accessSecret []byte
	refreshSecret []byte

	accessTTL time.Duration
	refreshTTL time.Duration

}

type TokenClaims struct {
	UserID int64 `json:"user_id"`
    jwt.RegisteredClaims
}

func NewTokenService(
	log *slog.Logger, 
	tokenRepository *TokenRepository,
	accessSecret string,
	refreshSecret string,
	accessTTL time.Duration,
	refreshTTL time.Duration,
) *TokenService {
	return &TokenService{
		tokenRepository: tokenRepository,
		log: log.With("component", "service", "entity", "refresh_token"),
		accessSecret: []byte(accessSecret),
		refreshSecret: []byte(refreshSecret),
		accessTTL: accessTTL,
		refreshTTL: refreshTTL,
	}
}

func (s *TokenService) GenerateTokens(userId int64) (TokenPair, error) {
	now := time.Now()

	accessClaims := TokenClaims{
		UserID: userId,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject: "access",
			IssuedAt: jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(s.accessTTL)),
		},
	}

	refreshClaims := TokenClaims{
		UserID: userId,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject: "refresh",
			IssuedAt: jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(s.refreshTTL)),
		},
	}

	accessToken, err := jwt.
		NewWithClaims(jwt.SigningMethodHS256, accessClaims).
		SignedString(s.accessSecret)

	if err != nil {
		s.log.Error("failed to generate access token", "user_id", userId, "error", err)
		return TokenPair{}, err
	}

	refreshToken, err := jwt.
		NewWithClaims(jwt.SigningMethodHS256, refreshClaims).
		SignedString(s.refreshSecret)

	if err != nil {
		s.log.Error("failed to generate refresh token", "user_id", userId, "error", err)
		return TokenPair{}, err
	}

	s.log.Debug("tokens generated", "user_id", userId)
	return TokenPair{
		AccesToken: accessToken,
		RefreshToken: refreshToken,
	}, nil
}

func (s *TokenService) ValidateAccessToken(tokenString string) (*TokenClaims, error) {
	return s.validateToken(tokenString, s.accessSecret)
}

func (s *TokenService) ValidateRefreshToken(tokenString string) (*TokenClaims, error) {
	return s.validateToken(tokenString, s.refreshSecret)
}

func (s *TokenService) validateToken(tokenString string, secret []byte) (*TokenClaims, error) {
	claims := &TokenClaims{}

	token, err := jwt.ParseWithClaims(
		tokenString, 
		claims,
		func(t *jwt.Token) (any, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, ErrInvalidToken
			}
			return secret, nil
		},
	)

	if err != nil {
		s.log.Debug("token validation failed", "error", err)
		return nil, err
	}

	if !token.Valid {
		return nil, ErrInvalidToken
	}

	return claims, nil
}

func (s *TokenService) SaveToken(ctx context.Context, userId int64, refreshToken string) error {
	hash, err := bcrypt.GenerateFromPassword(
		[]byte(refreshToken),
		bcrypt.DefaultCost,
	)

	if err != nil {
		s.log.Error("failed to hash refresh token", "user_id", userId, "error", err)
		return err
	}

	if err := s.tokenRepository.Save(ctx, userId, string(hash)); err != nil {
		s.log.Error("failed to save refresh token", "user_id", userId, "error", err)
		return err
	}

	s.log.Debug("refresh token saved", "user_id", userId)

	return nil
}

func (s *TokenService) RemoveToken(ctx context.Context, userId int64) error {
	if err := s.tokenRepository.DeleteByUserId(ctx, userId); err != nil {
		s.log.Error("failed to remove refresh token", "user_id", userId, "error", err)
		return err
	}

	s.log.Debug("refresh token removed", "user_id", userId)

	return nil
}

func (s *TokenService) FindToken(ctx context.Context, userId int64, refreshToken string) (*RefreshToken, error) {
	token, err := s.tokenRepository.FindByUserId(ctx, userId)

	if err != nil {
		s.log.Error("failed to find refresh token", "user_id", userId, "error", err)
		return nil, err
	}

	err = bcrypt.CompareHashAndPassword(
		[]byte(token.TokenHash),
		[]byte(refreshToken),
	)

	if err != nil {
		return nil, ErrInvalidToken
	}

	s.log.Debug("refresh token verified", "user_id", userId)

	return &token, nil
}