package middleware

import (
	"github.com/gin-gonic/gin"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestOriginPolicyPrecedesCookieMutation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(CheckOrigin([]string{"https://app.example.test"}))
	var calls int
	handler := func(c *gin.Context) { calls++; c.Header("Set-Cookie", "refreshToken=changed"); c.Status(204) }
	engine.POST("/api/refresh", handler)
	engine.POST("/api/logout", handler)
	engine.POST("/api/login", handler)
	for _, path := range []string{"/api/refresh", "/api/logout", "/api/login"} {
		for _, origin := range []string{"https://app.example.test", "https://evil.example", "null", ""} {
			req := httptest.NewRequest("POST", path, nil)
			if origin != "" {
				req.Header.Set("Origin", origin)
			}
			req.Host = "evil.example" // Host does not grant trust.
			response := httptest.NewRecorder()
			before := calls
			engine.ServeHTTP(response, req)
			allowed := origin == "https://app.example.test" || origin == "" && path == "/api/login"
			if allowed {
				if response.Code != 204 {
					t.Fatal("Trusted origin rejected")
				}
			} else if response.Code != 403 || calls != before || response.Header().Get("Set-Cookie") != "" {
				t.Fatal("Forbidden request mutated cookie")
			}
		}
	}
	req := httptest.NewRequest("POST", "/api/refresh", nil)
	req.Header.Add("Origin", "https://app.example.test")
	req.Header.Add("Origin", "https://evil.example")
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, req)
	if response.Code != http.StatusForbidden {
		t.Fatal("Ambiguous origins accepted")
	}
}

func TestRateLimiterConcurrentWindowAndBoundedCleanup(t *testing.T) {
	now := time.Now()
	limiter := NewRateLimiter(nil)
	limiter.now = func() time.Time { return now }
	limiter.capacity = 2
	rule := RateRule{Limit: 10, Window: time.Minute}
	var accepted atomic.Int32
	var workers sync.WaitGroup
	for range 100 {
		workers.Go(func() {
			if limiter.allow("one", rule) == 0 {
				accepted.Add(1)
			}
		})
	}
	workers.Wait()
	if accepted.Load() != 10 || limiter.allow("one", rule) != 60 {
		t.Fatal("Concurrent rate limit exceeded")
	}
	if limiter.allow("two", rule) != 0 || limiter.allow("three", rule) != 60 || len(limiter.entries) != 2 {
		t.Fatal("Capacity not bounded")
	}
	now = now.Add(time.Minute)
	if limiter.allow("three", rule) != 0 || len(limiter.entries) != 1 {
		t.Fatal("Expired entries not cleaned")
	}
}
