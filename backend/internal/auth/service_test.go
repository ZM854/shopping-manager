package auth

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
)

const testActivationToken = "12345678-1234-4123-8123-123456789abc"

func newTestUserService(log *slog.Logger, users userStore, tokens userTokens, mail activationMailer, url string) *UserService {
	return NewUserService(log, users, tokens, mail, url, DefaultActivationOptions())
}

func testLog() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

type usersFake struct {
	user           User
	err, updateErr error
	created        CreateUserRequest
	rotated        ActivationState
	activatedHash  string
	nextSend       time.Time
	inTx           bool
	txErr          error
	ctx            context.Context
	id             int64
}

func (f *usersFake) Create(ctx context.Context, r CreateUserRequest) (User, error) {
	f.ctx = ctx
	f.created = r
	if f.err != nil {
		return User{}, f.err
	}
	f.user = User{ID: 42, Name: r.Name, Email: r.Email, PasswordHash: r.PasswordHash}
	return f.user, nil
}
func (f *usersFake) GetByEmail(ctx context.Context, _ string) (User, error) {
	f.ctx = ctx
	return f.user, f.err
}
func (f *usersFake) GetById(ctx context.Context, id int64) (User, error) {
	f.ctx = ctx
	f.id = id
	return f.user, f.err
}
func (f *usersFake) Activate(ctx context.Context, hash string) error {
	f.ctx, f.activatedHash = ctx, hash
	return f.updateErr
}
func (f *usersFake) InActivationTx(ctx context.Context, fn func(activationTransaction) error) error {
	if f.txErr != nil {
		return f.txErr
	}
	f.inTx = true
	defer func() { f.inTx = false }()
	return fn(f)
}
func (f *usersFake) LockActivation(ctx context.Context, _ string) (ActivationAccount, error) {
	f.ctx = ctx
	return ActivationAccount{ID: 42, IsEmailVerified: f.user.IsEmailVerified, NextSendAt: f.nextSend}, f.err
}
func (f *usersFake) SaveActivation(ctx context.Context, id int64, state ActivationState) error {
	f.ctx, f.id, f.rotated = ctx, id, state
	return f.updateErr
}

type tokensFake struct {
	generateErr, saveErr, refreshErr, removeErr error
	generated, saved, removed                   int
	id                                          int64
	token                                       string
	ctx                                         context.Context
}

func (f *tokensFake) CreateSession(ctx context.Context, id int64) (TokenPair, error) {
	f.generated++
	f.id, f.ctx = id, ctx
	if f.generateErr != nil {
		return TokenPair{}, f.generateErr
	}
	f.saved++
	f.token = "test-refresh"
	return TokenPair{AccessToken: "test-access", RefreshToken: f.token}, f.saveErr
}
func (f *tokensFake) RefreshSession(ctx context.Context, raw string) (AuthResult, error) {
	f.ctx, f.token = ctx, raw
	return AuthResult{User: UserDTO{ID: 42}}, f.refreshErr
}
func (f *tokensFake) RevokeSession(ctx context.Context, raw string) error {
	f.ctx, f.token = ctx, raw
	f.removed++
	return f.removeErr
}

type mailFake struct {
	to, link string
	calls    int
	err      error
	deadline bool
	repo     *usersFake
}

func (f *mailFake) SendActivationMail(ctx context.Context, to, link string) error {
	f.to, f.link = to, link
	f.calls++
	_, f.deadline = ctx.Deadline()
	if f.repo != nil && f.repo.inTx {
		return errors.New("SMTP called during transaction")
	}
	return f.err
}

func TestRegistrationHashesPasswordAndSendsActivation(t *testing.T) {
	repo, tokens, mail := &usersFake{}, &tokensFake{}, &mailFake{}
	s := newTestUserService(testLog(), repo, tokens, mail, "https://example.test/api/activate")
	ctx := context.Background()
	resp, err := s.Registration(ctx, "Имя", "user@example.test", "password123")
	if err != nil {
		t.Fatal(err)
	}
	if bcrypt.CompareHashAndPassword([]byte(repo.created.PasswordHash), []byte("password123")) != nil || repo.created.PasswordHash == "password123" {
		t.Fatal("Password must be hashed")
	}
	if repo.created.Activation.TokenHash == "" || mail.to != "user@example.test" ||
		activationHash(mail.link[strings.LastIndex(mail.link, "/")+1:]) != repo.created.Activation.TokenHash ||
		mail.calls != 1 || !mail.deadline ||
		repo.created.Activation.ExpiresAt.Sub(repo.created.Activation.NextSendAt) != 24*time.Hour-time.Minute {
		t.Fatal("Incorrect activation email")
	}
	if !resp.ActivationRequired || repo.ctx != ctx || tokens.generated != 0 || tokens.saved != 0 {
		t.Fatal("Registration data or context lost")
	}
	data, _ := json.Marshal(resp)
	if strings.Contains(string(data), repo.created.PasswordHash) || strings.Contains(string(data), repo.created.Activation.TokenHash) {
		t.Fatal("Private user fields leaked into DTO")
	}
}
func TestRegistrationErrorsStopFollowingSteps(t *testing.T) {
	problem := errors.New("dependency failed")
	for _, stage := range []string{"hash", "duplicate", "repository", "mail"} {
		t.Run(stage, func(t *testing.T) {
			r, ts, m := &usersFake{}, &tokensFake{}, &mailFake{}
			password := "password123"
			want := problem
			switch stage {
			case "hash":
				password = strings.Repeat("x", 73)
				want = bcrypt.ErrPasswordTooLong
			case "duplicate":
				r.err = ErrUserAlreadyExist
				want = ErrUserAlreadyExist
			case "repository":
				r.err = problem
			case "mail":
				m.err = problem
				want = ErrActivationDelivery
			}
			_, err := newTestUserService(testLog(), r, ts, m, "http://example.test/activate").Registration(context.Background(), "Имя", "user@example.test", password)
			if !errors.Is(err, want) {
				t.Fatalf("error=%v, want %v", err, want)
			}
			if stage == "hash" || stage == "duplicate" || stage == "repository" {
				if m.calls != 0 || ts.generated != 0 || ts.saved != 0 {
					t.Fatal("Failed registration continued")
				}
			}
		})
	}
}
func TestLoginCredentialsActivationAndDependencyFailures(t *testing.T) {
	hash, _ := bcrypt.GenerateFromPassword([]byte("password123"), bcrypt.MinCost)
	problem := errors.New("database failed")
	for _, stage := range []string{"success", "missing", "repository", "password", "inactive", "generate", "save"} {
		t.Run(stage, func(t *testing.T) {
			r := &usersFake{user: User{ID: 42, Name: "Имя", Email: "user@example.test", PasswordHash: string(hash), IsEmailVerified: true}}
			ts := &tokensFake{}
			password := "password123"
			var want error
			switch stage {
			case "missing":
				r.err = ErrUserNotFound
				want = ErrInvalidCredentials
			case "repository":
				r.err = problem
				want = problem
			case "password":
				password = "wrong"
				want = ErrInvalidCredentials
			case "inactive":
				r.user.IsEmailVerified = false
				want = ErrUserNotActivated
			case "generate":
				ts.generateErr = problem
				want = problem
			case "save":
				ts.saveErr = problem
				want = problem
			}
			resp, err := newTestUserService(testLog(), r, ts, &mailFake{}, "").Login(context.Background(), "user@example.test", password)
			if !errors.Is(err, want) {
				t.Fatalf("error=%v want %v", err, want)
			}
			if want == nil && (resp.User.ID != 42 || ts.saved != 1 || ts.token != "test-refresh") {
				t.Fatal("Login did not persist session")
			}
			if stage == "missing" || stage == "repository" || stage == "password" || stage == "inactive" {
				if ts.generated != 0 || ts.saved != 0 {
					t.Fatal("Rejected login created tokens")
				}
			}
		})
	}
}
func TestActivationHashesTokenAndRejectsMalformed(t *testing.T) {
	repo := &usersFake{}
	service := newTestUserService(testLog(), repo, nil, nil, "")
	ctx := context.Background()
	for _, token := range []string{"", "bad", "12345678-1234-1123-8123-123456789abc"} {
		if err := service.Activate(ctx, token); !errors.Is(err, ErrInvalidActivation) || repo.activatedHash != "" {
			t.Fatal("Malformed token reached the repository")
		}
	}
	if err := service.Activate(ctx, testActivationToken); err != nil ||
		repo.activatedHash != activationHash(testActivationToken) || repo.ctx != ctx {
		t.Fatal("Activation must use only the token hash and request context")
	}
	for _, err := range []error{ErrInvalidActivation, errors.New("database failed")} {
		repo.updateErr = err
		if got := service.Activate(ctx, testActivationToken); !errors.Is(got, err) {
			t.Fatal("Activation lost error")
		}
	}
}

func TestResendActivationAndFailures(t *testing.T) {
	now := time.Now()
	problem := errors.New("dependency failed")
	for _, scenario := range []string{"success", "unknown", "verified", "cooldown", "lookup", "save", "transaction", "mail"} {
		t.Run(scenario, func(t *testing.T) {
			repo := &usersFake{}
			mail := &mailFake{repo: repo}
			service := newTestUserService(testLog(), repo, nil, mail, "https://example.test/activation")
			service.now = func() time.Time { return now }
			var want error
			switch scenario {
			case "unknown":
				repo.err = ErrUserNotFound
			case "verified":
				repo.user.IsEmailVerified = true
			case "cooldown":
				repo.nextSend = now.Add(30 * time.Second)
			case "lookup":
				repo.err, want = problem, problem
			case "save":
				repo.updateErr, want = problem, problem
			case "transaction":
				repo.txErr, want = problem, problem
			case "mail":
				mail.err, want = problem, ErrActivationDelivery
			}
			err := service.ResendActivation(context.Background(), "user@example.test")
			if scenario == "cooldown" {
				var cooldown *ActivationCooldownError
				if !errors.As(err, &cooldown) || cooldown.RetryAfter != 30 {
					t.Fatal("Cooldown lost")
				}
			} else if !errors.Is(err, want) {
				t.Fatalf("error=%v want=%v", err, want)
			}
			if scenario == "success" || scenario == "mail" {
				if mail.calls != 1 || !mail.deadline ||
					repo.rotated.TokenHash != activationHash(mail.link[strings.LastIndex(mail.link, "/")+1:]) ||
					!repo.rotated.ExpiresAt.Equal(now.Add(24*time.Hour)) ||
					!repo.rotated.NextSendAt.Equal(now.Add(time.Minute)) {
					t.Fatal("Resend must persist hashed expiring token before bounded SMTP")
				}
			} else if mail.calls != 0 {
				t.Fatal("Failed or neutral request sent email")
			}
		})
	}
}

func TestUserServiceDelegatesSessionOperations(t *testing.T) {
	problem := errors.New("storage failed")
	for _, failure := range []error{nil, problem} {
		tokens := &tokensFake{refreshErr: failure, removeErr: failure}
		service := newTestUserService(testLog(), &usersFake{}, tokens, nil, "")
		ctx := context.Background()
		result, err := service.Refresh(ctx, "refresh")
		if !errors.Is(err, failure) || tokens.token != "refresh" || tokens.ctx != ctx || result.User.ID != 42 {
			t.Fatal("Refresh lost result, error or context")
		}
		if err := service.Logout(ctx, "logout"); !errors.Is(err, failure) || tokens.token != "logout" || tokens.ctx != ctx {
			t.Fatal("Logout lost error or context")
		}
	}
}
