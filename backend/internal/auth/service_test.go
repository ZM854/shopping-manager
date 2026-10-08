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

func testLog() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

type usersFake struct {
	user           User
	err, updateErr error
	created        CreateUserRequest
	updated        UpdateUserRequest
	ctx            context.Context
	id             int64
}

func (f *usersFake) Create(ctx context.Context, r CreateUserRequest) (User, error) {
	f.ctx = ctx
	f.created = r
	if f.err != nil {
		return User{}, f.err
	}
	f.user = User{ID: 42, Name: r.Name, Email: r.Email, PasswordHash: r.PasswordHash, ActivationToken: r.ActivationToken}
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
func (f *usersFake) GetByActivationToken(ctx context.Context, _ string) (User, error) {
	f.ctx = ctx
	return f.user, f.err
}
func (f *usersFake) Update(ctx context.Context, id int64, r UpdateUserRequest) (User, error) {
	f.ctx = ctx
	f.id = id
	f.updated = r
	return f.user, f.updateErr
}
func (f *usersFake) GetAll(ctx context.Context) ([]User, error) {
	f.ctx = ctx
	return []User{f.user}, f.err
}

type tokensFake struct {
	generateErr, saveErr, validateErr, findErr, removeErr error
	generated, saved                                      int
	id                                                    int64
	token                                                 string
	ctx                                                   context.Context
}

func (f *tokensFake) GenerateTokens(id int64) (TokenPair, error) {
	f.generated++
	f.id = id
	return TokenPair{AccesToken: "test-access", RefreshToken: "test-refresh"}, f.generateErr
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
	f.ctx = ctx
	f.id = id
	return f.removeErr
}

type mailFake struct {
	to, link string
	calls    int
}

func (f *mailFake) SendActivationMail(_ context.Context, to, link string) error {
	f.to, f.link = to, link
	f.calls++
	return nil
}

func TestRegistrationHashesPasswordAndSendsActivation(t *testing.T) {
	repo, tokens, mail := &usersFake{}, &tokensFake{}, &mailFake{}
	s := NewUserService(testLog(), repo, tokens, mail, "https://example.test/api/activate")
	ctx := context.Background()
	resp, err := s.Registration(ctx, "Имя", "user@example.test", "password123")
	if err != nil {
		t.Fatal(err)
	}
	if bcrypt.CompareHashAndPassword([]byte(repo.created.PasswordHash), []byte("password123")) != nil || repo.created.PasswordHash == "password123" {
		t.Fatal("Password must be hashed")
	}
	if repo.created.ActivationToken == "" || mail.to != "user@example.test" || mail.link != "https://example.test/api/activate/"+repo.created.ActivationToken || mail.calls != 1 {
		t.Fatal("Incorrect activation email")
	}
	if resp.User.ID != 42 || resp.User.Name != "Имя" || repo.ctx != ctx || tokens.ctx != ctx || tokens.id != 42 || tokens.saved != 1 {
		t.Fatal("Registration data or context lost")
	}
	data, _ := json.Marshal(resp.User)
	if strings.Contains(string(data), repo.created.PasswordHash) || strings.Contains(string(data), repo.created.ActivationToken) {
		t.Fatal("Private user fields leaked into DTO")
	}
}
func TestRegistrationErrorsStopFollowingSteps(t *testing.T) {
	problem := errors.New("dependency failed")
	for _, stage := range []string{"hash", "duplicate", "repository", "generate", "save"} {
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
			case "generate":
				ts.generateErr = problem
			case "save":
				ts.saveErr = problem
			}
			_, err := NewUserService(testLog(), r, ts, m, "http://example.test/activate").Registration(context.Background(), "Имя", "user@example.test", password)
			if !errors.Is(err, want) {
				t.Fatalf("error=%v, want %v", err, want)
			}
			if stage == "hash" || stage == "duplicate" || stage == "repository" {
				if m.calls != 0 || ts.generated != 0 || ts.saved != 0 {
					t.Fatal("Failed registration continued")
				}
			}
			if stage == "generate" && ts.saved != 0 {
				t.Fatal("Failed generation persisted token")
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
			resp, err := NewUserService(testLog(), r, ts, &mailFake{}, "").Login(context.Background(), "user@example.test", password)
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
func TestActivationPreservesUserFields(t *testing.T) {
	problem := errors.New("write failed")
	for _, stage := range []string{"success", "invalid", "lookup", "update"} {
		t.Run(stage, func(t *testing.T) {
			r := &usersFake{user: User{ID: 42, Name: "Имя", Email: "user@example.test", PasswordHash: "test-hash"}}
			var want error
			switch stage {
			case "invalid":
				r.err = ErrUserNotFound
				want = ErrInvalidActivation
			case "lookup":
				r.err = problem
				want = problem
			case "update":
				r.updateErr = problem
				want = problem
			}
			err := NewUserService(testLog(), r, &tokensFake{}, &mailFake{}, "").Activate(context.Background(), "activation")
			if !errors.Is(err, want) {
				t.Fatalf("error=%v want %v", err, want)
			}
			if stage == "success" && (r.id != 42 || !r.updated.IsEmailVerified || r.updated.Name != r.user.Name || r.updated.Email != r.user.Email || r.updated.PasswordHash != r.user.PasswordHash) {
				t.Fatal("Activation changed unrelated user fields")
			}
		})
	}
}
func TestRefreshAndLogoutDependencyErrors(t *testing.T) {
	problem := errors.New("dependency failed")
	for _, stage := range []string{"success", "validate", "find", "user", "generate", "save"} {
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
			case "generate":
				ts.generateErr = problem
				want = problem
			case "save":
				ts.saveErr = problem
				want = problem
			}
			resp, err := NewUserService(testLog(), r, ts, &mailFake{}, "").Refresh(context.Background(), "old-refresh")
			if !errors.Is(err, want) {
				t.Fatalf("error=%v want %v", err, want)
			}
			if stage == "success" && (resp.User.ID != 42 || ts.saved != 1 || ts.token != "test-refresh") {
				t.Fatal("Refresh did not persist replacement")
			}
			if stage == "validate" || stage == "find" || stage == "user" {
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
		err := NewUserService(testLog(), &usersFake{}, ts, &mailFake{}, "").Logout(context.Background(), "refresh")
		if !errors.Is(err, want) {
			t.Fatal("Logout lost error")
		}
		if stage == "success" && ts.id != 42 {
			t.Fatal("Wrong revoked user")
		}
	}
	r := &usersFake{user: User{ID: 42}}
	users, err := NewUserService(testLog(), r, &tokensFake{}, &mailFake{}, "").GetAllUsers(context.Background())
	if err != nil || len(users) != 1 {
		t.Fatal("Internal user query failed")
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
	}{{pair.AccesToken, s.ValidateAccessToken, "access", time.Minute}, {pair.RefreshToken, s.ValidateRefreshToken, "refresh", time.Hour}} {
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
