//go:build integration

package integration_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ZM854/shopping-manager/backend/internal/auth"
)

func TestLegacyUnverifiedRefreshIsRevoked(t *testing.T) {
	t.Parallel()
	_, users, _, tokens, log := repoFixture(t)
	ctx := context.Background()
	user := createUser(t, users, 1)
	service := auth.NewTokenService(log, tokens, "test-access", "test-refresh", time.Minute, time.Hour)
	pair, err := service.GenerateTokens(user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.SaveToken(ctx, user.ID, pair.RefreshToken); err != nil {
		t.Fatal(err)
	}
	authService := auth.NewUserService(log, users, service, nil, "", auth.DefaultActivationOptions())
	if _, err := authService.Refresh(ctx, pair.RefreshToken); !errors.Is(err, auth.ErrUserNotActivated) {
		t.Fatal("Unverified legacy session must not refresh")
	}
	if _, err := tokens.FindByUserId(ctx, user.ID); !errors.Is(err, auth.ErrRefreshTokenNotFound) {
		t.Fatal("Rejected legacy session remains in the database")
	}
	if err := authService.Activate(ctx, testActivationToken(1)); err != nil {
		t.Fatal(err)
	}
	if _, err := authService.Refresh(ctx, pair.RefreshToken); !errors.Is(err, auth.ErrRefreshTokenNotFound) {
		t.Fatal("Activation must not restore the revoked legacy session")
	}
}
