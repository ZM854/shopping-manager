//go:build integration

package integration_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/ZM854/shopping-manager/backend/internal/auth"
	"github.com/ZM854/shopping-manager/backend/internal/testutil/testdb"
)

func TestActivationExpiresAndClearsOnlyActivationFields(t *testing.T) {
	t.Parallel()
	pool, users, _, _, log := repoFixture(t)
	ctx := context.Background()
	service := auth.NewUserService(log, users, nil, nil, "", auth.DefaultActivationOptions())
	for _, n := range []int{1, 2} {
		original := createUser(t, users, n)
		if err := service.Activate(ctx, testActivationToken(n)); err != nil {
			t.Fatal(err)
		}
		expected := original
		expected.IsEmailVerified = true
		loaded, err := users.GetById(ctx, original.ID)
		if err != nil || loaded != expected {
			t.Fatal("Activation changed profile or password")
		}
		var cleared bool
		if err := pool.QueryRow(ctx, `SELECT activation_token_hash IS NULL
			AND activation_expires_at IS NULL AND activation_next_send_at IS NULL
			FROM users WHERE id = $1`, original.ID).Scan(&cleared); err != nil || !cleared {
			t.Fatal("Consumed activation state must be NULL")
		}
		if err := service.Activate(ctx, testActivationToken(n)); !errors.Is(err, auth.ErrInvalidActivation) {
			t.Fatal("Consumed token accepted")
		}
	}
	expired := createUser(t, users, 3)
	if _, err := pool.Exec(ctx, `UPDATE users SET activation_expires_at = clock_timestamp() - interval '1 second' WHERE id = $1`, expired.ID); err != nil {
		t.Fatal(err)
	}
	if err := service.Activate(ctx, testActivationToken(3)); !errors.Is(err, auth.ErrInvalidActivation) {
		t.Fatal("Expired token accepted")
	}
	if err := service.Activate(ctx, ""); !errors.Is(err, auth.ErrInvalidActivation) {
		t.Fatal("Empty token accepted")
	}
}

func TestResendReplacesTokenAndReturnsNeutralForUnknownOrVerified(t *testing.T) {
	t.Parallel()
	_, users, _, _, log := repoFixture(t)
	ctx := context.Background()
	user := createUser(t, users, 1)
	mail := &activationCapture{}
	service := auth.NewUserService(log, users, nil, mail, "https://example.test/activation", auth.DefaultActivationOptions())
	if err := service.ResendActivation(ctx, user.Email); err != nil {
		t.Fatal(err)
	}
	var cooldown *auth.ActivationCooldownError
	if err := service.ResendActivation(ctx, user.Email); !errors.As(err, &cooldown) || cooldown.RetryAfter < 1 {
		t.Fatal("Repeated resend was not limited")
	}
	if err := service.Activate(ctx, testActivationToken(1)); !errors.Is(err, auth.ErrInvalidActivation) {
		t.Fatal("Replaced token accepted")
	}
	token := mail.link[len("https://example.test/activation/"):]
	if err := service.Activate(ctx, token); err != nil {
		t.Fatal(err)
	}
	mail.link = ""
	for _, email := range []string{user.Email, "unknown@example.test"} {
		if err := service.ResendActivation(ctx, email); err != nil || mail.link != "" {
			t.Fatal("Unknown or verified address must produce a neutral result without SMTP")
		}
	}
}

func TestActivationAndResendConcurrency(t *testing.T) {
	t.Parallel()
	for _, scenario := range []string{"two_resends", "two_activations", "resend_and_activation"} {
		t.Run(scenario, func(t *testing.T) {
			pool, users, _, _, log := repoFixture(t)
			ctx := context.Background()
			user := createUser(t, users, 1)
			mail := &activationCapture{}
			service := auth.NewUserService(log, users, nil, mail, "https://example.test/activation", auth.DefaultActivationOptions())
			start := make(chan struct{})
			results := make([]error, 2)
			var group sync.WaitGroup
			for i := range results {
				group.Add(1)
				go func(i int) {
					defer group.Done()
					<-start
					if scenario == "two_resends" || scenario == "resend_and_activation" && i == 0 {
						results[i] = service.ResendActivation(ctx, user.Email)
					} else {
						results[i] = service.Activate(ctx, testActivationToken(1))
					}
				}(i)
			}
			close(start)
			group.Wait()
			successes := 0
			for _, err := range results {
				if err == nil {
					successes++
				}
			}
			if scenario == "two_resends" {
				var cooldown *auth.ActivationCooldownError
				if successes != 1 || !(errors.As(results[0], &cooldown) || errors.As(results[1], &cooldown)) {
					t.Fatal("Only one resend may win")
				}
			} else if scenario == "two_activations" {
				if successes != 1 || !(errors.Is(results[0], auth.ErrInvalidActivation) || errors.Is(results[1], auth.ErrInvalidActivation)) {
					t.Fatal("Only one activation may consume a token")
				}
			} else {
				loaded, err := users.GetById(ctx, user.ID)
				if err != nil {
					t.Fatal(err)
				}
				var hash *string
				if err := pool.QueryRow(ctx, "SELECT activation_token_hash FROM users WHERE id = $1", user.ID).Scan(&hash); err != nil {
					t.Fatal(err)
				}
				if loaded.IsEmailVerified {
					if hash != nil || results[1] != nil {
						t.Fatal("Activation won but state was restored")
					}
				} else if hash == nil || !errors.Is(results[1], auth.ErrInvalidActivation) || results[0] != nil {
					t.Fatal("Resend must invalidate the earlier token")
				}
			}
		})
	}
}

func TestResendRollbackDoesNotSendEmail(t *testing.T) {
	t.Parallel()
	pool, users, _, _, log := repoFixture(t)
	ctx := context.Background()
	user := createUser(t, users, 1)
	mail := &activationCapture{}
	service := auth.NewUserService(log, users, nil, mail, "", auth.DefaultActivationOptions())
	migrationExec(t, pool, `CREATE FUNCTION reject_activation() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN RAISE EXCEPTION 'test-only failure'; END $$;
		CREATE TRIGGER reject_activation BEFORE UPDATE ON users FOR EACH ROW EXECUTE FUNCTION reject_activation()`)
	if err := service.ResendActivation(ctx, user.Email); err == nil || mail.link != "" {
		t.Fatal("Failed transaction sent email")
	}
	migrationExec(t, pool, "DROP TRIGGER reject_activation ON users")
	if err := service.Activate(ctx, testActivationToken(1)); err != nil {
		t.Fatal("Failed resend changed the previous token")
	}
}

type failingActivationMail struct{}

func (failingActivationMail) SendActivationMail(context.Context, string, string) error {
	return errors.New("test-only SMTP failure")
}

func TestRegistrationSMTPFailureKeepsPendingAccountRecoverable(t *testing.T) {
	t.Parallel()
	pool, users, _, _, log := repoFixture(t)
	ctx := context.Background()
	service := auth.NewUserService(log, users, nil, failingActivationMail{}, "https://example.test/activation", auth.DefaultActivationOptions())
	_, err := service.Registration(ctx, "Тест", "smtp@example.test", "password123")
	if !errors.Is(err, auth.ErrActivationDelivery) {
		t.Fatal("SMTP failure was ignored")
	}
	user, err := users.GetByEmail(ctx, "smtp@example.test")
	if err != nil || user.IsEmailVerified {
		t.Fatal("Failed SMTP must leave a pending account")
	}
	if _, err := service.Registration(ctx, "Другой", user.Email, "other-password"); !errors.Is(err, auth.ErrUserAlreadyExist) {
		t.Fatal("Registration must not replace the pending account")
	}
	unchanged, err := users.GetById(ctx, user.ID)
	if err != nil || unchanged != user {
		t.Fatal("Repeated registration changed the password or profile")
	}
	if _, err := pool.Exec(ctx, "UPDATE users SET activation_next_send_at = clock_timestamp() - interval '1 second' WHERE id = $1", user.ID); err != nil {
		t.Fatal(err)
	}
	mail := &activationCapture{}
	service = auth.NewUserService(log, users, nil, mail, "https://example.test/activation", auth.DefaultActivationOptions())
	if err := service.ResendActivation(ctx, user.Email); err != nil {
		t.Fatal(err)
	}
	if err := service.Activate(ctx, mail.link[len("https://example.test/activation/"):]); err != nil {
		t.Fatal("Account could not recover after SMTP failure")
	}
}

func TestActivationMigrationInvalidatesPlaintextLinks(t *testing.T) {
	t.Parallel()
	pool := testdb.New(t)
	testdb.ApplyMigrations(t, pool, 0, 5)
	id := migrationUser(t, pool, "migration@example.test")
	migrationExec(t, pool, "UPDATE users SET activation_token = 'old-test-token' WHERE id = $1", id)
	testdb.ApplyMigrations(t, pool, 5, 6)
	if got := migrationScalar(t, pool, "SELECT count(*)::text FROM users WHERE id = $1 AND activation_token_hash IS NULL", id); got != "1" {
		t.Fatal("Migration lost account or retained old activation")
	}
	if got := migrationScalar(t, pool, "SELECT count(*)::text FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = 'users' AND column_name = 'activation_token'"); got != "0" {
		t.Fatal("Plaintext activation column remains")
	}
}
