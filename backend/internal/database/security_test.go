package database

import (
	"bytes"
	"context"
	"errors"
	"github.com/ZM854/shopping-manager/backend/internal/config"
	"log/slog"
	"strings"
	"testing"
)

type failingPool struct{ closed bool }

func (p *failingPool) Ping(context.Context) error {
	return errors.New("private:postgres://user:synthetic-password@host/private-db")
}
func (p *failingPool) Close() { p.closed = true }
func TestPostgresEscapesConfigurationAndRedactsErrors(t *testing.T) {
	cfg := config.Config{DBHost: "::1", DBPort: "5432", DBUser: "user@/?:%", DBPassword: "synthetic:p@ss/?#% ' space", DBName: "database /?#%", DBSSLMode: "disable"}
	parsed, err := poolConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.ConnConfig.Host != cfg.DBHost || parsed.ConnConfig.User != cfg.DBUser || parsed.ConnConfig.Password != cfg.DBPassword || parsed.ConnConfig.Database != cfg.DBName {
		t.Fatal("DSN changed or injected configuration")
	}
	for _, port := range []string{"bad-synthetic-secret", "0", "65536"} {
		cfg.DBPort = port
		var output bytes.Buffer
		pool, err := NewPostgres(cfg, slog.New(slog.NewTextHandler(&output, nil)))
		if pool != nil || err == nil {
			t.Fatal("Invalid port accepted")
		}
		values := []string{cfg.DBUser, cfg.DBPassword, cfg.DBName, "bad-synthetic-secret"}
		for _, value := range values {
			if strings.Contains(err.Error()+output.String(), value) {
				t.Fatal("Connection details leaked")
			}
		}
	}
	pool := &failingPool{}
	err = checkConnection(context.Background(), pool)
	if err == nil || !pool.closed || strings.Contains(err.Error(), "synthetic-password") {
		t.Fatal("Failed ping must close the pool and redact the cause")
	}
}
