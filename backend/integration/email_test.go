//go:build integration

package integration_test

import (
	"context"
	"errors"
	"github.com/ZM854/shopping-manager/backend/internal/auth"
	"github.com/ZM854/shopping-manager/backend/internal/testutil/testdb"
	"os"
	"strings"
	"testing"
	"time"
)

func TestEmailNormalizationMigrationPreservesAccountsAndPrivateData(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	testdb.ApplyMigrations(t, pool, 0, 7)
	owner := migrationUser(t, pool, "\u0085  FIRST@Example.Test \u00a0")
	migrationUser(t, pool, "second@example.test")
	migrationExec(t, pool, `INSERT INTO shopping_lists (owner_id,name) VALUES ($1,'Список');
 `, owner)
	migrationExec(t, pool, `INSERT INTO shopping_items (list_id,name,quantity,unit,is_marked) VALUES (1,'Молоко',1.125,'л',true)`)
	migrationExec(t, pool, `INSERT INTO refresh_tokens(session_id,user_id,token_hash,expires_at) VALUES ('12345678-1234-4123-8123-123456789abc',$1,repeat('b',64),clock_timestamp()+interval '1 hour')`, owner)
	migrationExec(t, pool, `INSERT INTO shopping_idempotency(owner_id,operation,key,request_hash,resource_id,response,expires_at) VALUES ($1,'lists','normalization-key-0001',repeat('a',64),1,'{"id":"1"}',clock_timestamp()+interval '24 hours')`, owner)
	snapshot := `SELECT jsonb_build_object(
  'users',(SELECT jsonb_agg(to_jsonb(u)-'email' ORDER BY id) FROM users u),
  'lists',(SELECT jsonb_agg(to_jsonb(l) ORDER BY id) FROM shopping_lists l),
  'items',(SELECT jsonb_agg(to_jsonb(i) ORDER BY id) FROM shopping_items i),
  'sessions',(SELECT jsonb_agg(to_jsonb(s) ORDER BY session_id) FROM refresh_tokens s),
  'idempotency',(SELECT jsonb_agg(to_jsonb(k) ORDER BY key) FROM shopping_idempotency k)
 )::text`
	before := migrationScalar(t, pool, snapshot)
	testdb.ApplyMigrations(t, pool, 7, 8)
	if migrationScalar(t, pool, snapshot) != before {
		t.Fatal("Normalization changed owners, passwords, activation, sessions or shopping data")
	}
	if migrationScalar(t, pool, `SELECT email FROM users WHERE id=$1`, owner) != "first@example.test" {
		t.Fatal("Email normalization mismatch")
	}
	if _, err := pool.Exec(context.Background(), `INSERT INTO users(name,email,password_hash) VALUES ('Имя','FIRST@example.test','hash')`); err == nil {
		t.Fatal("Database accepts noncanonical address")
	}
	if _, err := pool.Exec(context.Background(), `INSERT INTO users(name,email,password_hash) VALUES ('Имя','first@example.test','hash')`); err == nil {
		t.Fatal("Normalized uniqueness lost")
	}
}

func TestEmailConflictStopsMigrationWithoutChangesOrAddressDisclosure(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	testdb.ApplyMigrations(t, pool, 0, 7)
	migrationUser(t, pool, "secret-owner@example.test")
	migrationUser(t, pool, " SECRET-OWNER@Example.Test ")
	migrationUser(t, pool, " Other@Example.Test ")
	before := migrationScalar(t, pool, `SELECT jsonb_agg(to_jsonb(u) ORDER BY id)::text FROM users u`)
	sql, err := os.ReadFile("../migrations/000008_normalize_email.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	_, err = pool.Exec(context.Background(), string(sql))
	if err == nil || !strings.Contains(err.Error(), "email normalization conflict") || strings.Contains(err.Error(), "secret-owner") {
		t.Fatal("Collision must stop migration with a safe diagnostic")
	}
	if migrationScalar(t, pool, `SELECT jsonb_agg(to_jsonb(u) ORDER BY id)::text FROM users u`) != before {
		t.Fatal("Failed migration changed accounts")
	}
}

func TestNormalizedRegistrationLoginAndResend(t *testing.T) {
	t.Parallel()
	pool, users, _, tokens, log := repoFixture(t)
	mail := &activationCapture{}
	service := auth.NewUserService(log, users, auth.NewTokenService(log, tokens, "test-access", "test-refresh", time.Minute, time.Hour), mail, "https://client.example.test/activation", auth.DefaultActivationOptions())
	ctx := context.Background()
	if _, err := service.Registration(ctx, "  Имя  ", " USER@Example.Test ", "password123"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Registration(ctx, "Другое", "user@example.test", "password123"); !errors.Is(err, auth.ErrUserAlreadyExist) {
		t.Fatal("Case variation created a second account")
	}
	user, err := users.GetByEmail(ctx, "user@example.test")
	if err != nil || user.Name != "Имя" {
		t.Fatal("Canonical account missing")
	}
	var cooldown *auth.ActivationCooldownError
	if err := service.ResendActivation(ctx, " USER@EXAMPLE.TEST "); !errors.As(err, &cooldown) {
		t.Fatal("Resend lookup was not normalized")
	}
	token := mail.link[strings.LastIndex(mail.link, "/")+1:]
	if err := service.Activate(ctx, token); err != nil {
		t.Fatal(err)
	}
	result, err := service.Login(ctx, " USER@Example.TEST ", "password123")
	if err != nil || result.User.Email != "user@example.test" || result.User.ID == "" {
		t.Fatal("Login normalization mismatch")
	}
	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM users`).Scan(&count); err != nil || count != 1 {
		t.Fatal("Registration duplicated account")
	}
}
