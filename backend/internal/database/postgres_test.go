package database

import (
	"github.com/ZM854/shopping-manager/backend/internal/config"
	"io"
	"log/slog"
	"testing"
)

func TestInvalidPostgresConfig(t *testing.T) {
	pool, err := NewPostgres(config.Config{DBHost: "127.0.0.1", DBPort: "invalid-port", DBUser: "test", DBName: "test", DBSSLMode: "disable"}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err == nil || pool != nil {
		t.Fatal("Invalid DSN accepted")
	}
}
