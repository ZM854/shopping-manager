//go:build integration

package integration_test

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ZM854/shopping-manager/backend/internal/auth"
	"github.com/ZM854/shopping-manager/backend/internal/testutil/testdb"
	"golang.org/x/crypto/bcrypt"
)

func TestAuthUpgradeFromVersion5PreservesDataAndRecoversAccounts(t *testing.T) {
	t.Parallel()
	f := newShoppingFixture(t) // Real schema 5, including persisted idempotency results.
	ctx := context.Background()
	hash, err := bcrypt.GenerateFromPassword([]byte("password123"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	migrationExec(t, f.pool, `UPDATE users SET email = ' FIRST@Example.Test ',
  password_hash = $2, is_email_verified = true WHERE id = $1`, f.first, string(hash))
	migrationExec(t, f.pool, `UPDATE users SET email = ' SECOND@Example.Test ',
  password_hash = $2, activation_token = $3 WHERE id = $1`, f.second, string(hash), testActivationToken(2))
	issuer := auth.NewTokenService(sessionTestLog(), nil,
		"upgrade-test-access-key", "upgrade-test-refresh-key", time.Minute, time.Hour)
	oldSession, err := issuer.GenerateTokens(f.first)
	if err != nil {
		t.Fatal(err)
	}
	migrationExec(t, f.pool, `INSERT INTO refresh_tokens(user_id, token_hash)
  VALUES ($1, $3), ($2, 'old-second-refresh')`, f.first, f.second, sessionHash(oldSession.RefreshToken))
	list := f.list(t, f.first, "upgrade-list-0001")
	item := f.item(t, f.first, list.ID, "upgrade-item-0001")
	otherList := f.list(t, f.second, "upgrade-list-0002")
	snapshot := `SELECT jsonb_build_object(
  'accounts', (SELECT jsonb_agg(jsonb_build_object(
   'id', id, 'name', name, 'password_hash', password_hash,
   'is_email_verified', is_email_verified) ORDER BY id) FROM users),
  'lists', (SELECT jsonb_agg(to_jsonb(l) ORDER BY id) FROM shopping_lists l),
  'items', (SELECT jsonb_agg(to_jsonb(i) ORDER BY id) FROM shopping_items i),
  'replays', (SELECT jsonb_agg(to_jsonb(r) ORDER BY owner_id, operation, key)
   FROM shopping_idempotency r)
 )::text`
	before := migrationScalar(t, f.pool, snapshot)
	// Match the CLI: each migration commits separately.
	for version := 5; version < 8; version++ {
		testdb.ApplyMigrations(t, f.pool, version, version+1)
	}
	if migrationScalar(t, f.pool, snapshot) != before {
		t.Fatal("Upgrade changed accounts, ownership, shopping data or saved POST responses")
	}
	if migrationScalar(t, f.pool, `SELECT count(*)::text FROM refresh_tokens`) != "0" {
		t.Fatal("Old refresh sessions survived upgrade")
	}
	if migrationScalar(t, f.pool, `SELECT count(*)::text FROM users
  WHERE activation_token_hash IS NOT NULL OR activation_expires_at IS NOT NULL`) != "0" {
		t.Fatal("Old activation links survived upgrade")
	}

	// The original response must still replay, without creating a second list or item.
	replayList := f.list(t, f.first, "upgrade-list-0001")
	replayItem := f.item(t, f.first, list.ID, "upgrade-item-0001")
	if !reflect.DeepEqual(replayList, list) || !reflect.DeepEqual(replayItem, item) {
		t.Fatal("Upgrade broke persisted POST replay")
	}
	shoppingStatus(t, f.request(f.first, "GET", shoppingPath(otherList.ID), "", ""), 404)
	if migrationScalar(t, f.pool, snapshot) != before {
		t.Fatal("Replay modified resources after upgrade")
	}

	users := auth.NewUserRepository(f.pool)
	tokens := auth.NewTokenService(sessionTestLog(), auth.NewTokenRepository(f.pool, nil),
		"upgrade-test-access-key", "upgrade-test-refresh-key", time.Minute, time.Hour)
	mail := &activationCapture{}
	service := auth.NewUserService(sessionTestLog(), users, tokens, mail,
		"https://example.test/activation", auth.DefaultActivationOptions())
	// Even a correctly signed, unexpired JWT cannot recreate a removed session.
	if _, err := service.Refresh(ctx, oldSession.RefreshToken); !errors.Is(err, auth.ErrRefreshTokenNotFound) {
		t.Fatal("Old session survived upgrade or was recreated")
	}
	if err := service.Activate(ctx, testActivationToken(2)); !errors.Is(err, auth.ErrInvalidActivation) {
		t.Fatal("Old activation link accepted")
	}
	if _, err := service.Login(ctx, " SECOND@Example.Test ", "password123"); !errors.Is(err, auth.ErrUserNotActivated) {
		t.Fatal("Pending account bypassed activation")
	}
	if err := service.ResendActivation(ctx, " SECOND@Example.Test "); err != nil {
		t.Fatal(err)
	}
	token := mail.link[strings.LastIndex(mail.link, "/")+1:]
	if err := service.Activate(ctx, token); err != nil {
		t.Fatal(err)
	}
	for _, account := range []struct {
		id    int64
		email string
	}{{f.first, "first@example.test"}, {f.second, "second@example.test"}} {
		user, err := users.GetById(ctx, account.id)
		if err != nil || user.Email != account.email || user.PasswordHash != string(hash) || !user.IsEmailVerified {
			t.Fatal("Account did not recover after upgrade")
		}
		session, err := service.Login(ctx, strings.ToUpper(account.email), "password123")
		if err != nil || session.User.ID != fmt.Sprint(account.id) {
			t.Fatal("Login after upgrade failed or lost string ID")
		}
		refreshed, err := service.Refresh(ctx, session.Tokens.RefreshToken)
		if err != nil {
			t.Fatal(err)
		}
		if err := service.Logout(ctx, refreshed.Tokens.RefreshToken); err != nil {
			t.Fatal(err)
		}
		if _, err := service.Refresh(ctx, refreshed.Tokens.RefreshToken); !errors.Is(err, auth.ErrRefreshTokenNotFound) {
			t.Fatal("Logout after upgrade did not revoke the session")
		}
	}
}
