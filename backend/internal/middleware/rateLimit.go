package middleware

import (
	"math"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

type RateRule struct {
	Limit  int
	Window time.Duration
}
type rateEntry struct {
	count   int
	expires time.Time
}
type RateLimiter struct {
	mu          sync.Mutex
	entries     map[string]rateEntry
	rules       map[string]RateRule
	capacity    int
	nextCleanup time.Time
	now         func() time.Time
}

func NewRateLimiter(rules map[string]RateRule) *RateLimiter {
	return &RateLimiter{
		entries: make(map[string]rateEntry), rules: rules,
		capacity: 10000, now: time.Now,
	}
}

func (l *RateLimiter) allow(key string, rule RateRule) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	if !now.Before(l.nextCleanup) {
		for key, entry := range l.entries {
			if !now.Before(entry.expires) {
				delete(l.entries, key)
			}
		}
		l.nextCleanup = now.Add(time.Minute)
	}
	entry, found := l.entries[key]
	if found && !now.Before(entry.expires) {
		delete(l.entries, key)
		found = false
	}
	if !found {
		if len(l.entries) >= l.capacity {
			return 60
		}
		entry = rateEntry{expires: now.Add(rule.Window)}
	}
	if entry.count >= rule.Limit {
		return max(1, int(math.Ceil(entry.expires.Sub(now).Seconds())))
	}
	entry.count++
	l.entries[key] = entry
	return 0
}

func (l *RateLimiter) Handle() gin.HandlerFunc {
	return func(c *gin.Context) {
		if rule, found := l.rules[c.Request.URL.Path]; found && c.Request.Method == http.MethodPost {
			if retry := l.allow(c.Request.URL.Path+":"+c.ClientIP(), rule); retry > 0 {
				c.Header("Cache-Control", "no-store")
				c.Header("Retry-After", strconv.Itoa(retry))
				c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
					"code": "rate_limited", "message": "Слишком много запросов. Повторите позже.",
				})
				return
			}
		}
		c.Next()
	}
}
