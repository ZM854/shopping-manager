//go:build integration

package integration_test

import (
	"context"
	"errors"
	"testing"

	"github.com/ZM854/shopping-manager/backend/internal/auth"
)

func TestActivationClearsUniqueTokensForMultipleUsers(t *testing.T) {
	t.Parallel()
	pool, users, _, _, log := repoFixture(t)
	ctx := context.Background()
	service := auth.NewUserService(log, users, nil, nil, "http://example.test/api/activate")
	first := createUser(t, users, 1)
	second := createUser(t, users, 2)

	for _, original := range []auth.User{first, second} {
		if err := service.Activate(ctx, original.ActivationToken); err != nil {
			t.Fatalf("activate user %d: %v", original.ID, err)
		}
		expected := original
		expected.IsEmailVerified = true
		expected.ActivationToken = ""

		byID, err := users.GetById(ctx, original.ID)
		if err != nil || byID != expected {
			t.Fatalf("activated user fields differ when read by ID: %v", err)
		}
		byEmail, err := users.GetByEmail(ctx, original.Email)
		if err != nil || byEmail != expected {
			t.Fatalf("activated user fields differ when read by email: %v", err)
		}
		err = service.Activate(ctx, original.ActivationToken)
		if !errors.Is(err, auth.ErrInvalidActivation) {
			t.Fatalf("used activation token accepted: %v", err)
		}
	}

	all, err := users.GetAll(ctx)
	if err != nil || len(all) != 2 {
		t.Fatalf("read activated users: count %d, error %v", len(all), err)
	}
	for _, user := range all {
		if !user.IsEmailVerified || user.ActivationToken != "" {
			t.Fatal("activated user still has an activation token")
		}
	}
	var cleared int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM users WHERE activation_token IS NULL
	`).Scan(&cleared); err != nil || cleared != 2 {
		t.Fatalf("cleared tokens must be NULL: count %d, error %v", cleared, err)
	}
	if err := service.Activate(ctx, ""); !errors.Is(err, auth.ErrInvalidActivation) {
		t.Fatalf("empty activation token accepted: %v", err)
	}
}

func TestActivationWithLegacyEmptyToken(t *testing.T) {
	t.Parallel()
	pool, users, _, _, log := repoFixture(t)
	ctx := context.Background()
	first := createUser(t, users, 1)
	second := createUser(t, users, 2)
	if _, err := pool.Exec(ctx, `
		UPDATE users
		SET is_email_verified = true, activation_token = ''
		WHERE id = $1
	`, first.ID); err != nil {
		t.Fatal(err)
	}

	service := auth.NewUserService(log, users, nil, nil, "http://example.test/api/activate")
	if err := service.Activate(ctx, ""); !errors.Is(err, auth.ErrInvalidActivation) {
		t.Fatalf("legacy empty token accepted: %v", err)
	}
	if err := service.Activate(ctx, second.ActivationToken); err != nil {
		t.Fatalf("legacy empty token prevents another activation: %v", err)
	}
	loaded, err := users.GetById(ctx, first.ID)
	first.IsEmailVerified = true
	first.ActivationToken = ""
	if err != nil || loaded != first {
		t.Fatalf("legacy activated user fields changed: %v", err)
	}
}
