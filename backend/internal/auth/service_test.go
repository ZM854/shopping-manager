package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
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
	generateErr, saveErr, validateErr, findErr, removeErr error
	generated, saved, removed                             int
	id                                                    int64
	token                                                 string
	ctx                                                   context.Context
}

func (f *tokensFake) GenerateTokens(id int64) (TokenPair, error) {
	f.generated++
	f.id = id
	return TokenPair{AccessToken: "test-access", RefreshToken: "test-refresh"}, f.generateErr
}
func (f *tokensFake) SaveToken(ctx context.Context, id int64, s string) error {
	f.saved++
	f.ctx = ctx
	f.id = id
	f.token = s
	return f.saveErr
}
func (f *tokensFake) ValidateRefreshToken(s string) (*TokenClaims, error) {
	f.token = s
	return &TokenClaims{UserID: 42}, f.validateErr
}
func (f *tokensFake) FindToken(ctx context.Context, id int64, s string) (*RefreshToken, error) {
	f.ctx = ctx
	f.id = id
	f.token = s
	return &RefreshToken{UserID: id}, f.findErr
}
func (f *tokensFake) RemoveToken(ctx context.Context, id int64) error {
	f.removed++
	f.ctx = ctx
	f.id = id
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

func TestRefreshAndLogoutDependencyErrors(t *testing.T) {
	problem := errors.New("dependency failed")
	for _, stage := range []string{"success", "validate", "find", "user", "inactive", "revoke", "already_revoked", "generate", "save"} {
		t.Run(stage, func(t *testing.T) {
			r := &usersFake{user: User{ID: 42, IsEmailVerified: true}}
			ts := &tokensFake{}
			var want error
			switch stage {
			case "validate":
				ts.validateErr = problem
				want = problem
			case "find":
				ts.findErr = problem
				want = problem
			case "user":
				r.err = problem
				want = problem
			case "inactive", "revoke", "already_revoked":
				r.user.IsEmailVerified = false
				want = ErrUserNotActivated
				if stage == "revoke" {
					ts.removeErr = problem
					want = problem
				}
				if stage == "already_revoked" {
					ts.removeErr = ErrRefreshTokenNotFound
				}
			case "generate":
				ts.generateErr = problem
				want = problem
			case "save":
				ts.saveErr = problem
				want = problem
			}
			resp, err := newTestUserService(testLog(), r, ts, &mailFake{}, "").Refresh(context.Background(), "old-refresh")
			if !errors.Is(err, want) {
				t.Fatalf("error=%v want %v", err, want)
			}
			if stage == "success" && (resp.User.ID != 42 || ts.saved != 1 || ts.token != "test-refresh") {
				t.Fatal("Refresh did not persist replacement")
			}
			if stage == "inactive" || stage == "revoke" || stage == "already_revoked" {
				if ts.removed != 1 || ts.id != 42 {
					t.Fatal("Inactive user's legacy refresh must be revoked")
				}
			}
			if stage != "success" && stage != "generate" && stage != "save" {
				if ts.generated != 0 || ts.saved != 0 {
					t.Fatal("Invalid session continued refresh")
				}
			}
		})
	}
	for _, stage := range []string{"success", "validate", "remove"} {
		ts := &tokensFake{}
		var want error
		if stage == "validate" {
			ts.validateErr = problem
			want = problem
		}
		if stage == "remove" {
			ts.removeErr = problem
			want = problem
		}
		err := newTestUserService(testLog(), &usersFake{}, ts, &mailFake{}, "").Logout(context.Background(), "refresh")
		if !errors.Is(err, want) {
			t.Fatal("Logout lost error")
		}
		if stage == "success" && ts.id != 42 {
			t.Fatal("Wrong revoked user")
		}
	}
}

type tokenRepoFake struct {
	token RefreshToken
	err   error
	id    int64
	hash  string
}

func (f *tokenRepoFake) Save(_ context.Context, id int64, hash string) error {
	f.id, f.hash = id, hash
	return f.err
}
func (f *tokenRepoFake) FindByUserId(_ context.Context, id int64) (RefreshToken, error) {
	f.id = id
	return f.token, f.err
}
func (f *tokenRepoFake) DeleteByUserId(_ context.Context, id int64) error { f.id = id; return f.err }
func TestJWTSignatureExpiryAndSecrets(t *testing.T) {
	s := NewTokenService(testLog(), &tokenRepoFake{}, "test-access-secret", "test-refresh-secret", time.Minute, time.Hour)
	pair, err := s.GenerateTokens(42)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		token    string
		validate func(string) (*TokenClaims, error)
		subject  string
		ttl      time.Duration
	}{{pair.AccessToken, s.ValidateAccessToken, "access", time.Minute}, {pair.RefreshToken, s.ValidateRefreshToken, "refresh", time.Hour}} {
		claims, err := tc.validate(tc.token)
		if err != nil || claims.UserID != 42 || claims.Subject != tc.subject || claims.ExpiresAt.Sub(claims.IssuedAt.Time) != tc.ttl {
			t.Fatalf("Wrong JWT claims: %v", err)
		}
	}
	for _, token := range []string{"", "malformed", pair.RefreshToken} {
		if _, err := s.ValidateAccessToken(token); err == nil {
			t.Fatal("Invalid access token accepted")
		}
	}
	expired, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, TokenClaims{UserID: 42, RegisteredClaims: jwt.RegisteredClaims{ExpiresAt: jwt.NewNumericDate(time.Now().Add(-time.Hour))}}).SignedString([]byte("test-access-secret"))
	if _, err := s.ValidateAccessToken(expired); !errors.Is(err, jwt.ErrTokenExpired) {
		t.Fatal("Expired token accepted")
	}
	unsigned, _ := jwt.NewWithClaims(jwt.SigningMethodNone, TokenClaims{UserID: 42}).SignedString(jwt.UnsafeAllowNoneSignatureType)
	if _, err := s.ValidateAccessToken(unsigned); err == nil {
		t.Fatal("Unsigned token accepted")
	}
}
func TestTokenStorageUsesHashAndRejectsMismatch(t *testing.T) {
	repo := &tokenRepoFake{}
	s := NewTokenService(testLog(), repo, "a", "r", time.Minute, time.Hour)
	ctx := context.Background()
	if err := s.SaveToken(ctx, 42, "refresh-secret"); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte("refresh-secret"))
	want := hex.EncodeToString(sum[:])
	if repo.id != 42 || repo.hash != want {
		t.Fatal("Token must be stored hashed")
	}
	repo.token = RefreshToken{UserID: 42, TokenHash: want}
	token, err := s.FindToken(ctx, 42, "refresh-secret")
	if err != nil || token.UserID != 42 {
		t.Fatal("Stored token not found")
	}
	if _, err := s.FindToken(ctx, 42, "different"); !errors.Is(err, ErrInvalidToken) {
		t.Fatal("Mismatch accepted")
	}
	problem := errors.New("storage failed")
	repo.err = problem
	if err := s.SaveToken(ctx, 42, "x"); !errors.Is(err, problem) {
		t.Fatal("Save lost error")
	}
	if _, err := s.FindToken(ctx, 42, "x"); !errors.Is(err, problem) {
		t.Fatal("Find lost error")
	}
	if err := s.RemoveToken(ctx, 42); !errors.Is(err, problem) {
		t.Fatal("Delete lost error")
	}
	repo.err = nil
	if err := s.RemoveToken(ctx, 42); err != nil {
		t.Fatal(err)
	}
}
