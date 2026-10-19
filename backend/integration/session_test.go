//go:build integration

package integration_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ZM854/shopping-manager/backend/internal/auth"
	"github.com/ZM854/shopping-manager/backend/internal/testutil/testdb"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

func sessionTestLog() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func sessionFixture(t *testing.T) (*pgxpool.Pool, *auth.TokenService, *auth.UserService, auth.User) {
	t.Helper()
	pool, users, _, tokens, log := repoFixture(t)
	user := createUser(t, users, 1)
	hash, err := bcrypt.GenerateFromPassword([]byte("password123"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(context.Background(), `UPDATE users SET password_hash = $2 WHERE id = $1`, user.ID, string(hash)); err != nil {
		t.Fatal(err)
	}
	if err := users.Activate(context.Background(), testActivationState(testActivationToken(1)).TokenHash); err != nil {
		t.Fatal(err)
	}
	service := auth.NewTokenService(log, tokens, "test-access", "test-refresh", time.Minute, time.Hour)
	return pool, service, auth.NewUserService(log, users, service, nil, "", auth.DefaultActivationOptions()), user
}

func sessionHash(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}

func TestConcurrentRefreshAndIndependentBrowserSessions(t *testing.T) {
	t.Parallel()
	pool, tokens, service, user := sessionFixture(t)
	ctx := context.Background()
	first, err := service.Login(ctx, user.Email, "password123")
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.Login(ctx, user.Email, "password123")
	if err != nil {
		t.Fatal(err)
	}
	if first.Tokens.SessionID == second.Tokens.SessionID {
		t.Fatal("Each login needs its own session")
	}
	var created, expiry, used time.Time
	var hash string
	if err := pool.QueryRow(ctx, `SELECT created_at, expires_at, last_used_at, token_hash FROM refresh_tokens WHERE session_id = $1`, first.Tokens.SessionID).Scan(&created, &expiry, &used, &hash); err != nil {
		t.Fatal(err)
	}
	if hash != sessionHash(first.Tokens.RefreshToken) || !expiry.Equal(first.Tokens.RefreshExpiresAt) {
		t.Fatal("Session must store hash and absolute expiry")
	}
	start := make(chan struct{})
	type outcome struct {
		result auth.AuthResult
		err    error
	}
	results := make(chan outcome, 2)
	for range 2 {
		go func() {
			<-start
			result, err := service.Refresh(ctx, first.Tokens.RefreshToken)
			results <- outcome{result, err}
		}()
	}
	close(start)
	var winner auth.AuthResult
	successes, failures := 0, 0
	for range 2 {
		out := <-results
		if out.err == nil {
			successes++
			winner = out.result
		} else if errors.Is(out.err, auth.ErrInvalidToken) {
			failures++
		} else {
			t.Fatal(out.err)
		}
	}
	if successes != 1 || failures != 1 {
		t.Fatal("Only one concurrent rotation may succeed")
	}
	var afterCreated, afterExpiry, afterUsed time.Time
	if err := pool.QueryRow(ctx, `SELECT created_at, expires_at, last_used_at, token_hash FROM refresh_tokens WHERE session_id = $1`, first.Tokens.SessionID).Scan(&afterCreated, &afterExpiry, &afterUsed, &hash); err != nil {
		t.Fatal(err)
	}
	if !created.Equal(afterCreated) || !expiry.Equal(afterExpiry) || afterUsed.Before(used) || hash != sessionHash(winner.Tokens.RefreshToken) {
		t.Fatal("Rotation must atomically replace the hash and preserve lifetime")
	}
	// A lost response is intentionally unrecoverable with the old secret.
	if _, err := service.Refresh(ctx, first.Tokens.RefreshToken); !errors.Is(err, auth.ErrInvalidToken) {
		t.Fatal("Lost response must not enable replay")
	}
	if err := service.Logout(ctx, first.Tokens.RefreshToken); err != nil {
		t.Fatal(err)
	}
	winner, err = service.Refresh(ctx, winner.Tokens.RefreshToken)
	if err != nil {
		t.Fatal("Stale logout revoked the rotated session")
	}
	if err := service.Logout(ctx, winner.Tokens.RefreshToken); err != nil {
		t.Fatal(err)
	}
	if err := service.Logout(ctx, winner.Tokens.RefreshToken); err != nil {
		t.Fatal("Repeat logout must succeed")
	}
	if _, err := service.Refresh(ctx, winner.Tokens.RefreshToken); !errors.Is(err, auth.ErrRefreshTokenNotFound) {
		t.Fatal("Logout session resurrected")
	}
	if _, err := service.Refresh(ctx, second.Tokens.RefreshToken); err != nil {
		t.Fatal("Logout affected the other browser")
	}
	if _, err := tokens.ValidateRefreshToken(winner.Tokens.RefreshToken); err != nil {
		t.Fatal("The refusal must come from session storage")
	}
}

func TestRefreshRollbackExpiryAndUserVerification(t *testing.T) {
	t.Parallel()
	pool, _, service, user := sessionFixture(t)
	ctx := context.Background()
	session, err := service.Login(ctx, user.Email, "password123")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		CREATE FUNCTION fail_rotation() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN RAISE EXCEPTION 'test rotation failure'; END $$;
		CREATE TRIGGER fail_rotation BEFORE UPDATE ON refresh_tokens
		FOR EACH ROW EXECUTE FUNCTION fail_rotation()`); err != nil {
		t.Fatal(err)
	}
	if result, err := service.Refresh(ctx, session.Tokens.RefreshToken); err == nil || result.Tokens.RefreshToken != "" {
		t.Fatal("Failed transaction exposed new tokens")
	}
	var hash string
	if err := pool.QueryRow(ctx, `SELECT token_hash FROM refresh_tokens WHERE session_id = $1`, session.Tokens.SessionID).Scan(&hash); err != nil || hash != sessionHash(session.Tokens.RefreshToken) {
		t.Fatal("Rollback lost original hash")
	}
	if _, err := pool.Exec(ctx, `DROP TRIGGER fail_rotation ON refresh_tokens`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE users SET is_email_verified = false WHERE id = $1`, user.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Refresh(ctx, session.Tokens.RefreshToken); !errors.Is(err, auth.ErrUserNotActivated) {
		t.Fatal("Unverified account refreshed")
	}
	if _, err := pool.Exec(ctx, `UPDATE users SET is_email_verified = true WHERE id = $1`, user.ID); err != nil {
		t.Fatal(err)
	}
	rotated, err := service.Refresh(ctx, session.Tokens.RefreshToken)
	if err != nil {
		t.Fatal("Original token must remain usable after rollback")
	}
	other, err := service.Login(ctx, user.Email, "password123")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE refresh_tokens SET created_at = clock_timestamp() - interval '2 hours', expires_at = clock_timestamp() - interval '1 hour' WHERE session_id = $1`, rotated.Tokens.SessionID); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Refresh(ctx, rotated.Tokens.RefreshToken); !errors.Is(err, auth.ErrRefreshTokenNotFound) {
		t.Fatal("DB-expired session refreshed despite a valid JWT")
	}
	repo := auth.NewTokenRepository(pool, nil)
	if removed, err := repo.CleanupExpired(ctx); err != nil || removed != 1 {
		t.Fatal("Expiry cleanup failed")
	}
	if _, err := service.Refresh(ctx, other.Tokens.RefreshToken); err != nil {
		t.Fatal("Cleanup deleted an active session")
	}
}

func TestLogoutWinsAgainstWaitingRefresh(t *testing.T) {
	pool, _, service, user := sessionFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	session, err := service.Login(ctx, user.Email, "password123")
	if err != nil {
		t.Fatal(err)
	}
	// Hold the deletion uncommitted while a real refresh waits on the same row.
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	if _, err := tx.Exec(ctx, `DELETE FROM refresh_tokens WHERE session_id = $1 AND token_hash = $2`, session.Tokens.SessionID, sessionHash(session.Tokens.RefreshToken)); err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() { _, err := service.Refresh(ctx, session.Tokens.RefreshToken); result <- err }()
	for {
		var waiting bool
		if err := pool.QueryRow(ctx, `SELECT EXISTS (
			SELECT 1 FROM pg_stat_activity
			WHERE datname = current_database() AND wait_event_type = 'Lock'
			AND query LIKE '%SELECT rt.session_id::text%')`).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting {
			break
		}
		select {
		case err := <-result:
			t.Fatalf("Refresh did not wait for logout: %v", err)
		case <-ctx.Done():
			t.Fatal("Refresh did not reach the locked session")
		case <-time.After(10 * time.Millisecond):
		}
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-result; !errors.Is(err, auth.ErrRefreshTokenNotFound) {
		t.Fatalf("Waiting refresh resurrected logout: %v", err)
	}
	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM refresh_tokens`).Scan(&count); err != nil || count != 0 {
		t.Fatal("Refresh recreated deleted row")
	}
}

func TestSessionHTTPFailuresDoNotOverwriteCookie(t *testing.T) {
	pool, _, service, user := sessionFixture(t)
	ctx := context.Background()
	login, err := service.Login(ctx, user.Email, "password123")
	if err != nil {
		t.Fatal(err)
	}
	h := auth.NewAuthHandler(sessionTestLog(), service, false)
	engine := gin.New()
	engine.POST("/refresh", h.Refresh)
	engine.POST("/logout", h.Logout)
	request := func(path, token string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("POST", path, strings.NewReader(""))
		if token != "" {
			req.AddCookie(&http.Cookie{Name: "refreshToken", Value: token})
		}
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)
		return w
	}
	rotated := request("/refresh", login.Tokens.RefreshToken)
	if rotated.Code != 200 {
		t.Fatal("Refresh failed")
	}
	newCookie := rotated.Result().Cookies()[0]
	stale := request("/refresh", login.Tokens.RefreshToken)
	if stale.Code != 401 || stale.Header().Get("Set-Cookie") != "" {
		t.Fatal("Stale response damaged the new cookie")
	}
	if request("/refresh", newCookie.Value).Code != 200 {
		t.Fatal("Stale refusal damaged the new session")
	}
	for _, cookie := range []string{"", "malformed", login.Tokens.RefreshToken} {
		w := request("/logout", cookie)
		if w.Code != 200 || len(w.Result().Cookies()) != 1 || w.Result().Cookies()[0].MaxAge != -1 {
			t.Fatal("Idempotent logout must clear its cookie")
		}
	}
	// Get a current token before closing the pool, so the DB failure is observable.
	current, err := service.Login(ctx, user.Email, "password123")
	if err != nil {
		t.Fatal(err)
	}
	pool.Close()
	w := request("/logout", current.Tokens.RefreshToken)
	if w.Code != 500 || w.Header().Get("Set-Cookie") != "" {
		t.Fatal("DB failure must not report confirmed logout")
	}
}

func TestSessionMigrationRevokesOldTokensAndPreservesContent(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	testdb.ApplyMigrations(t, pool, 0, 6)
	users := auth.NewUserRepository(pool)
	user := createUser(t, users, 1)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `INSERT INTO refresh_tokens (user_id, token_hash) VALUES ($1, 'old-format-hash')`, user.ID); err != nil {
		t.Fatal(err)
	}
	var listID int64
	if err := pool.QueryRow(ctx, `INSERT INTO shopping_lists (owner_id, name) VALUES ($1, 'Сохранённый список') RETURNING id`, user.ID).Scan(&listID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO shopping_items (list_id, name, quantity, unit) VALUES ($1, 'Молоко', 1.25, 'л')`, listID); err != nil {
		t.Fatal(err)
	}
	testdb.ApplyMigrations(t, pool, 6, 7)
	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM refresh_tokens`).Scan(&count); err != nil || count != 0 {
		t.Fatal("Old refresh format must be invalidated")
	}
	loaded, err := users.GetById(ctx, user.ID)
	if err != nil || loaded != user {
		t.Fatal("Migration changed account")
	}
	var name, quantity string
	if err := pool.QueryRow(ctx, `SELECT i.name, i.quantity::text FROM shopping_items i JOIN shopping_lists l ON l.id = i.list_id WHERE l.owner_id = $1 AND l.id = $2`, user.ID, listID).Scan(&name, &quantity); err != nil || name != "Молоко" || quantity != "1.25" {
		t.Fatal("Migration changed shopping content")
	}
	service := auth.NewTokenService(sessionTestLog(), auth.NewTokenRepository(pool, nil), "access", "refresh", time.Minute, time.Hour)
	for range 2 {
		if _, err := service.CreateSession(ctx, user.ID); err != nil {
			t.Fatal("Multiple sessions unavailable after migration")
		}
	}
}
