package config

import (
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestLoadDefaultsAndOverrides(t *testing.T) {
	t.Chdir(t.TempDir()) // Load не читает настоящий .env.
	for _, key := range []string{"APP_ENV", "SERVER_PORT", "PUBLIC_API_URL", "FRONTEND_URL", "DB_HOST", "DB_PORT", "DB_USER", "DB_PASSWORD", "DB_NAME", "DB_SSLMODE", "JWT_ACCESS_SECRET", "JWT_REFRESH_SECRET", "JWT_ACCESS_TTL", "JWT_REFRESH_TTL", "SMTP_HOST", "SMTP_PORT", "SMTP_USER", "SMTP_PASSWORD", "SMTP_FROM", "SMTP_AUTH_ENABLED", "SMTP_TLS_REQUIRED"} {
		t.Setenv(key, "")
	}
	t.Setenv("SERVER_PORT", ":8080")
	cfg := Load()
	if cfg.PublicAPIURL != "http://localhost:8080/api" || cfg.FrontendURL != "http://localhost:5173" || cfg.JWTAccessTTL != 15*time.Minute || cfg.JWTRefreshTTL != 30*24*time.Hour || !cfg.SMTPAuthEnabled || !cfg.SMTPTLSRequired {
		t.Fatal("Incorrect defaults")
	}
	for key, value := range map[string]string{"APP_ENV": "test", "PUBLIC_API_URL": "https://example.test/api///", "FRONTEND_URL": "https://example.test///", "DB_HOST": "db", "DB_PORT": "5432", "DB_USER": "test", "DB_PASSWORD": "fake-password", "DB_NAME": "test", "DB_SSLMODE": "disable", "JWT_ACCESS_SECRET": "fake-access", "JWT_REFRESH_SECRET": "fake-refresh", "JWT_ACCESS_TTL": "2m", "JWT_REFRESH_TTL": "24h", "SMTP_HOST": "smtp", "SMTP_PORT": "1025", "SMTP_USER": "test", "SMTP_PASSWORD": "fake-smtp", "SMTP_FROM": "sender@example.test", "SMTP_AUTH_ENABLED": "false", "SMTP_TLS_REQUIRED": "false"} {
		t.Setenv(key, value)
	}
	cfg = Load()
	if cfg.AppENV != "test" || cfg.PublicAPIURL != "https://example.test/api" || cfg.FrontendURL != "https://example.test" || cfg.JWTAccessTTL != 2*time.Minute || cfg.JWTRefreshTTL != 24*time.Hour || cfg.SMTPAuthEnabled || cfg.SMTPTLSRequired || cfg.DBHost != "db" || cfg.DBPassword != "fake-password" || cfg.SMTPFrom != "sender@example.test" || cfg.JWTAccessSecret != "fake-access" {
		t.Fatal("Overrides not applied")
	}
}
func TestDurationAndBoolParsing(t *testing.T) {
	for _, value := range []string{"", "invalid"} {
		if parseDuration(value, time.Hour) != time.Hour {
			t.Fatal("Missing fallback")
		}
	}
	if parseDuration("90s", time.Hour) != 90*time.Second || !parseBool("", true) || !parseBool("true", false) || parseBool("false", true) {
		t.Fatal("Incorrect parsing")
	}
}
func TestInvalidBoolExits(t *testing.T) {
	if os.Getenv("TEST_INVALID_BOOL_CHILD") == "1" {
		parseBool("invalid", false)
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestInvalidBoolExits$")
	cmd.Env = append(os.Environ(), "TEST_INVALID_BOOL_CHILD=1")
	output, err := cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(output), "invalid boolean configuration") {
		t.Fatal("Invalid bool must fail explicitly")
	}
}
