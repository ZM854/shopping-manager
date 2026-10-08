package middleware

import (
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http/httptest"
	"testing"

	"github.com/ZM854/shopping-manager/backend/internal/auth"
	"github.com/gin-gonic/gin"
)

type validatorFunc func(string) (*auth.TokenClaims, error)

func (f validatorFunc) ValidateAccessToken(s string) (*auth.TokenClaims, error) { return f(s) }

func TestAuthorizationStopsInvalidRequests(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		header string
		err    error
		want   int
		calls  int
	}{{"", nil, 401, 0}, {"Basic abc", nil, 401, 0}, {"Bearer valid", nil, 200, 1}, {"Bearer invalid", auth.ErrInvalidToken, 401, 1}, {"Bearer failed", errors.New("validator failure"), 401, 1}} {
		t.Run(tc.header, func(t *testing.T) {
			calls, handled := 0, false
			r := gin.New()
			r.Use(NewAuthMiddleware(validatorFunc(func(token string) (*auth.TokenClaims, error) {
				calls++
				if token != tc.header[len(bearerPrefix):] {
					t.Fatal("Wrong token")
				}
				return &auth.TokenClaims{UserID: 42}, tc.err
			})).HandleAuth())
			r.GET("/private", func(c *gin.Context) {
				handled = true
				id, _ := c.Get("userID")
				if id != int64(42) {
					t.Fatal("Wrong userID")
				}
				c.Status(200)
			})
			req := httptest.NewRequest("GET", "/private", nil)
			req.Header.Set("Authorization", tc.header)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			if w.Code != tc.want || calls != tc.calls || handled != (tc.want == 200) {
				t.Fatalf("status=%d calls=%d handled=%v", w.Code, calls, handled)
			}
		})
	}
}
func TestRequestLoggerStatusAndSafePath(t *testing.T) {
	for _, tc := range []struct {
		status int
		level  string
	}{{200, "INFO"}, {400, "WARN"}, {500, "ERROR"}} {
		var buf bytes.Buffer
		r := gin.New()
		r.Use(RequestLogger(slog.New(slog.NewJSONHandler(&buf, nil))))
		r.GET("/resource/:id", func(c *gin.Context) {
			if tc.status == 500 {
				_ = c.Error(errors.New("test error"))
			}
			c.Status(tc.status)
		})
		req := httptest.NewRequest("GET", "/resource/private-value?token=test-secret", nil)
		req.Header.Set("Authorization", "Bearer test-token")
		r.ServeHTTP(httptest.NewRecorder(), req)
		var entry map[string]any
		if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
			t.Fatal(err)
		}
		if entry["level"] != tc.level || entry["path"] != "/resource/:id" || entry["status"] != float64(tc.status) {
			t.Fatalf("Unexpected entry: %s", buf.String())
		}
		if bytes.Contains(buf.Bytes(), []byte("test-secret")) || bytes.Contains(buf.Bytes(), []byte("test-token")) || bytes.Contains(buf.Bytes(), []byte("private-value")) {
			t.Fatal("Request secrets leaked")
		}
	}
}
