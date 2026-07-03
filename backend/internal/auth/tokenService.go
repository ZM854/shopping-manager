package auth

import (
	"log/slog"
)

type TokenService struct {
	log *slog.Logger
	tokenRepository *TokenRepository
}

func NewTokenService(log *slog.Logger, tokenRepository *TokenRepository) *TokenService {
	return &TokenService{
		tokenRepository: tokenRepository,
		log: log.With("component", "service", "entity", "refresh_token"),
	}
}

