package auth

import (
	"context"
	"errors"
	"testing"
	"time"
)

type sessionStoreFake struct {
	session                                                    RefreshToken
	user                                                       User
	createErr, txErr, lockErr, rotateErr, deleteErr, commitErr error
	locks, rotations, deletions                                int
	deletedUser                                                int64
	deletedSID, deletedHash                                    string
	inTx                                                       bool
}

func (f *sessionStoreFake) Create(_ context.Context, session RefreshToken) error {
	f.session = session
	return f.createErr
}
func (f *sessionStoreFake) Delete(_ context.Context, userID int64, sid, hash string) error {
	f.deletions++
	f.deletedUser, f.deletedSID, f.deletedHash = userID, sid, hash
	return f.deleteErr
}
func (f *sessionStoreFake) InSessionTx(_ context.Context, fn func(sessionTransaction) error) error {
	if f.txErr != nil {
		return f.txErr
	}
	f.inTx = true
	defer func() { f.inTx = false }()
	if err := fn(f); err != nil {
		return err
	}
	return f.commitErr
}
func (f *sessionStoreFake) Lock(_ context.Context, userID int64, sid string) (RefreshToken, User, error) {
	f.locks++
	if !f.inTx || userID != f.session.UserID || sid != f.session.SessionID {
		return RefreshToken{}, User{}, ErrRefreshTokenNotFound
	}
	return f.session, f.user, f.lockErr
}
func (f *sessionStoreFake) Rotate(_ context.Context, old RefreshToken, hash string) error {
	f.rotations++
	if !f.inTx || old.TokenHash != f.session.TokenHash {
		return ErrInvalidToken
	}
	if f.rotateErr != nil {
		return f.rotateErr
	}
	f.session.TokenHash = hash
	return nil
}

func TestSessionsRotateWithinSameSecondWithoutExtendingLifetime(t *testing.T) {
	repo := &sessionStoreFake{user: User{ID: 42, IsEmailVerified: true}}
	s := NewTokenService(testLog(), repo, "access", "refresh", time.Minute, time.Hour)
	now := time.Now().Truncate(time.Second)
	s.now = func() time.Time { return now }
	first, err := s.CreateSession(context.Background(), 42)
	if err != nil {
		t.Fatal(err)
	}
	if repo.session.TokenHash != refreshHash(first.RefreshToken) || repo.session.TokenHash == first.RefreshToken {
		t.Fatal("Store only the refresh hash")
	}
	previous := first
	for range 3 {
		result, err := s.RefreshSession(context.Background(), previous.RefreshToken)
		if err != nil {
			t.Fatal(err)
		}
		claims, err := s.ValidateRefreshToken(result.Tokens.RefreshToken)
		oldClaims, _ := s.ValidateRefreshToken(previous.RefreshToken)
		if err != nil || claims.ID == oldClaims.ID || claims.SessionID != oldClaims.SessionID ||
			!claims.ExpiresAt.Equal(first.RefreshExpiresAt) || !claims.IssuedAt.Equal(oldClaims.IssuedAt.Time) ||
			result.Tokens.RefreshToken == previous.RefreshToken {
			t.Fatal("Same-second rotation must change jti/hash and preserve sid/expiry")
		}
		if _, err := s.RefreshSession(context.Background(), previous.RefreshToken); !errors.Is(err, ErrInvalidToken) {
			t.Fatal("Used refresh accepted")
		}
		previous = result.Tokens
	}
	now = now.Add(30 * time.Minute)
	result, err := s.RefreshSession(context.Background(), previous.RefreshToken)
	if err != nil || !result.Tokens.RefreshExpiresAt.Equal(first.RefreshExpiresAt) {
		t.Fatal("Sliding expiry is forbidden")
	}
	now = first.RefreshExpiresAt.Add(-10 * time.Second)
	nearExpiry, err := s.RefreshSession(context.Background(), result.Tokens.RefreshToken)
	if err != nil {
		t.Fatal(err)
	}
	accessClaims, err := s.ValidateAccessToken(nearExpiry.Tokens.AccessToken)
	if err != nil || !accessClaims.ExpiresAt.Equal(first.RefreshExpiresAt) {
		t.Fatal("Access must not outlive the absolute session expiry")
	}
	now = first.RefreshExpiresAt
	if _, err := s.RefreshSession(context.Background(), result.Tokens.RefreshToken); !errors.Is(err, ErrInvalidToken) {
		t.Fatal("Expired JWT accepted")
	}
}

func TestSessionFailuresNeverReturnUncommittedTokens(t *testing.T) {
	problem := errors.New("storage failed")
	for _, scenario := range []string{"create", "transaction", "lock", "hash", "expiry", "inactive", "rotate", "commit"} {
		t.Run(scenario, func(t *testing.T) {
			repo := &sessionStoreFake{user: User{ID: 42, IsEmailVerified: true}}
			s := NewTokenService(testLog(), repo, "access", "refresh", time.Minute, time.Hour)
			if scenario == "create" {
				repo.createErr = problem
				pair, err := s.CreateSession(context.Background(), 42)
				if !errors.Is(err, problem) || pair.RefreshToken != "" {
					t.Fatal("Failed insert exposed tokens")
				}
				return
			}
			pair, err := s.CreateSession(context.Background(), 42)
			if err != nil {
				t.Fatal(err)
			}
			want := problem
			switch scenario {
			case "transaction":
				repo.txErr = problem
			case "lock":
				repo.lockErr = problem
			case "hash":
				repo.session.TokenHash = "other"
				want = ErrInvalidToken
			case "expiry":
				repo.session.ExpiresAt = repo.session.ExpiresAt.Add(time.Second)
				want = ErrInvalidToken
			case "inactive":
				repo.user.IsEmailVerified = false
				want = ErrUserNotActivated
			case "rotate":
				repo.rotateErr = problem
			case "commit":
				repo.commitErr = problem
			}
			result, err := s.RefreshSession(context.Background(), pair.RefreshToken)
			if !errors.Is(err, want) || result.Tokens.RefreshToken != "" {
				t.Fatalf("Uncommitted refresh: %v", err)
			}
			if scenario != "rotate" && scenario != "commit" && repo.rotations != 0 {
				t.Fatal("Rejected session rotated")
			}
		})
	}
}

func TestLogoutMatchesExactSessionAndIsIdempotent(t *testing.T) {
	repo := &sessionStoreFake{}
	s := NewTokenService(testLog(), repo, "access", "refresh", time.Minute, time.Hour)
	pair, err := s.CreateSession(context.Background(), 42)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.RevokeSession(context.Background(), pair.RefreshToken); err != nil ||
		repo.deletedUser != 42 || repo.deletedSID != pair.SessionID || repo.deletedHash != refreshHash(pair.RefreshToken) {
		t.Fatal("Logout must match owner, sid and exact token hash")
	}
	problem := errors.New("storage failed")
	repo.deleteErr = problem
	if err := s.RevokeSession(context.Background(), pair.RefreshToken); !errors.Is(err, problem) {
		t.Fatal("DB failure suppressed")
	}
	before := repo.deletions
	for _, raw := range []string{"", "broken", pair.AccessToken} {
		if err := s.RevokeSession(context.Background(), raw); err != nil {
			t.Fatal("Unusable cookie must allow local logout")
		}
	}
	s.now = func() time.Time { return pair.RefreshExpiresAt }
	if err := s.RevokeSession(context.Background(), pair.RefreshToken); err != nil || repo.deletions != before {
		t.Fatal("Expired cookie must not touch DB")
	}
}
