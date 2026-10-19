package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

var ErrInvalidToken = errors.New("invalid token")

type sessionTransaction interface {
	Lock(context.Context, int64, string) (RefreshToken, User, error)
	Rotate(context.Context, RefreshToken, string) error
}

type tokenStore interface {
	Create(context.Context, RefreshToken) error
	Delete(context.Context, int64, string, string) error
	InSessionTx(context.Context, func(sessionTransaction) error) error
}

type TokenService struct {
	log                         *slog.Logger
	tokenRepository             tokenStore
	accessSecret, refreshSecret []byte
	accessTTL, refreshTTL       time.Duration
	now                         func() time.Time
}

type TokenClaims struct {
	UserID    int64  `json:"user_id"`
	SessionID string `json:"sid,omitempty"`
	jwt.RegisteredClaims
}

func NewTokenService(
	log *slog.Logger,
	repo tokenStore,
	accessSecret, refreshSecret string,
	accessTTL, refreshTTL time.Duration,
) *TokenService {
	return &TokenService{
		log:             log.With("component", "service", "entity", "refresh_token"),
		tokenRepository: repo,
		accessSecret:    []byte(accessSecret),
		refreshSecret:   []byte(refreshSecret),
		accessTTL:       accessTTL,
		refreshTTL:      refreshTTL,
		now:             time.Now,
	}
}

// GenerateTokens issues a new pair; only CreateSession persists a login.
func (s *TokenService) GenerateTokens(userID int64) (TokenPair, error) {
	// PostgreSQL expiry and JWT NumericDate use the same second precision.
	expiresAt := s.now().Add(s.refreshTTL).Truncate(time.Second)
	return s.generateTokens(userID, uuid.NewString(), expiresAt)
}

func (s *TokenService) generateTokens(userID int64, sessionID string, expiresAt time.Time) (TokenPair, error) {
	now := s.now()
	if userID <= 0 || !expiresAt.After(now) {
		return TokenPair{}, ErrInvalidToken
	}
	accessExpiresAt := now.Add(s.accessTTL)
	if expiresAt.Before(accessExpiresAt) {
		accessExpiresAt = expiresAt
	}
	accessClaims := TokenClaims{
		UserID: userID,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "access",
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(accessExpiresAt),
		},
	}
	refreshClaims := TokenClaims{
		UserID:    userID,
		SessionID: sessionID,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "refresh",
			ID:        uuid.NewString(),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(expiresAt),
		},
	}
	access, err := jwt.NewWithClaims(jwt.SigningMethodHS256, accessClaims).SignedString(s.accessSecret)
	if err != nil {
		return TokenPair{}, err
	}
	refresh, err := jwt.NewWithClaims(jwt.SigningMethodHS256, refreshClaims).SignedString(s.refreshSecret)
	if err != nil {
		return TokenPair{}, err
	}
	return TokenPair{
		AccessToken:      access,
		RefreshToken:     refresh,
		SessionID:        sessionID,
		RefreshExpiresAt: expiresAt,
	}, nil
}

func (s *TokenService) ValidateAccessToken(token string) (*TokenClaims, error) {
	return s.validateToken(token, s.accessSecret, "access")
}

func (s *TokenService) ValidateRefreshToken(token string) (*TokenClaims, error) {
	return s.validateToken(token, s.refreshSecret, "refresh")
}

func (s *TokenService) validateToken(raw string, secret []byte, subject string) (*TokenClaims, error) {
	claims := &TokenClaims{}
	token, err := jwt.ParseWithClaims(
		raw, claims,
		func(_ *jwt.Token) (any, error) { return secret, nil },
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithExpirationRequired(),
		jwt.WithSubject(subject),
		jwt.WithIssuedAt(),
		jwt.WithTimeFunc(s.now),
	)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidToken, err)
	}
	if !token.Valid || claims.UserID <= 0 || claims.IssuedAt == nil ||
		!claims.ExpiresAt.After(claims.IssuedAt.Time) {
		return nil, ErrInvalidToken
	}
	if subject == "refresh" && (!validSessionUUID(claims.SessionID) || !validSessionUUID(claims.ID)) {
		return nil, ErrInvalidToken
	}
	return claims, nil
}

func validSessionUUID(value string) bool {
	id, err := uuid.Parse(value)
	return err == nil && id.Version() == 4 && id.Variant() == uuid.RFC4122 && id.String() == value
}

func refreshHash(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func (s *TokenService) CreateSession(ctx context.Context, userID int64) (TokenPair, error) {
	pair, err := s.GenerateTokens(userID)
	if err != nil {
		return TokenPair{}, err
	}
	err = s.tokenRepository.Create(ctx, RefreshToken{
		UserID: userID, SessionID: pair.SessionID,
		TokenHash: refreshHash(pair.RefreshToken), ExpiresAt: pair.RefreshExpiresAt,
	})
	if err != nil {
		return TokenPair{}, err
	}
	s.log.Debug("refresh session created", "user_id", userID)
	return pair, nil
}

func (s *TokenService) RefreshSession(ctx context.Context, raw string) (AuthResult, error) {
	claims, err := s.ValidateRefreshToken(raw)
	if err != nil {
		return AuthResult{}, err
	}
	var result AuthResult
	err = s.tokenRepository.InSessionTx(ctx, func(tx sessionTransaction) error {
		session, user, err := tx.Lock(ctx, claims.UserID, claims.SessionID)
		if err != nil {
			return err
		}
		if session.TokenHash != refreshHash(raw) || !session.ExpiresAt.Equal(claims.ExpiresAt.Time) {
			return ErrInvalidToken
		}
		if !user.IsEmailVerified {
			return ErrUserNotActivated
		}
		pair, err := s.generateTokens(user.ID, session.SessionID, session.ExpiresAt)
		if err != nil {
			return err
		}
		if err := tx.Rotate(ctx, session, refreshHash(pair.RefreshToken)); err != nil {
			return err
		}
		result = AuthResult{User: NewUserDTO(user), Tokens: pair}
		return nil
	})
	if err != nil {
		return AuthResult{}, err
	}
	return result, nil
}

func (s *TokenService) RevokeSession(ctx context.Context, raw string) error {
	claims, err := s.ValidateRefreshToken(raw)
	if errors.Is(err, ErrInvalidToken) {
		// Missing, malformed and expired cookies are already unusable.
		return nil
	}
	if err != nil {
		return err
	}
	return s.tokenRepository.Delete(ctx, claims.UserID, claims.SessionID, refreshHash(raw))
}
