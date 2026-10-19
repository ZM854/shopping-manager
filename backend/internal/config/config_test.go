package config

import (
	"strings"
	"testing"
	"time"
)

func configTestEnv(t *testing.T) {
	t.Helper()
	t.Chdir(t.TempDir()) // Never load the real .env.
	for _, key := range []string{
		"APP_ENV", "SERVER_PORT", "PUBLIC_API_URL", "FRONTEND_URL",
		"DB_HOST", "DB_PORT", "DB_USER", "DB_PASSWORD", "DB_NAME", "DB_SSLMODE",
		"JWT_ACCESS_SECRET", "JWT_REFRESH_SECRET", "JWT_ACCESS_TTL", "JWT_REFRESH_TTL",
		"ACTIVATION_TTL", "ACTIVATION_RESEND_INTERVAL", "ACTIVATION_MAIL_TIMEOUT",
		"SMTP_HOST", "SMTP_PORT", "SMTP_USER", "SMTP_PASSWORD", "SMTP_FROM",
		"SMTP_AUTH_ENABLED", "SMTP_TLS_REQUIRED",
	} {
		t.Setenv(key, "")
	}
	t.Setenv("SERVER_PORT", ":8080")
	t.Setenv("JWT_ACCESS_SECRET", strings.Repeat("a", 32))
	t.Setenv("JWT_REFRESH_SECRET", strings.Repeat("r", 32))
}

func TestLoadDefaultsAndOverrides(t *testing.T) {
	configTestEnv(t)
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.PublicAPIURL != "http://localhost:8080/api" ||
		cfg.FrontendURL != "http://localhost:5173" ||
		cfg.JWTAccessTTL != 15*time.Minute || cfg.JWTRefreshTTL != 30*24*time.Hour ||
		cfg.ActivationTTL != 24*time.Hour || cfg.ActivationResendInterval != time.Minute ||
		cfg.ActivationMailTimeout != 10*time.Second ||
		!cfg.SMTPAuthEnabled || !cfg.SMTPTLSRequired {
		t.Fatal("Incorrect defaults")
	}
	for key, value := range map[string]string{
		"APP_ENV": "test", "PUBLIC_API_URL": "https://example.test/api///",
		"FRONTEND_URL": "https://example.test///", "DB_HOST": "db", "DB_PORT": "5432",
		"DB_USER": "test", "DB_PASSWORD": "fake-password", "DB_NAME": "test",
		"DB_SSLMODE": "disable", "JWT_ACCESS_TTL": "2m", "JWT_REFRESH_TTL": "24h",
		"ACTIVATION_TTL": "2h", "ACTIVATION_RESEND_INTERVAL": "30s", "ACTIVATION_MAIL_TIMEOUT": "5s",
		"SMTP_HOST": "smtp", "SMTP_PORT": "1025", "SMTP_USER": "test",
		"SMTP_PASSWORD": "fake-smtp", "SMTP_FROM": "sender@example.test",
		"SMTP_AUTH_ENABLED": "false", "SMTP_TLS_REQUIRED": "false",
	} {
		t.Setenv(key, value)
	}
	cfg, err = Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AppENV != "test" || cfg.PublicAPIURL != "https://example.test/api" ||
		cfg.FrontendURL != "https://example.test" || cfg.JWTAccessTTL != 2*time.Minute ||
		cfg.JWTRefreshTTL != 24*time.Hour || cfg.SMTPAuthEnabled || cfg.SMTPTLSRequired ||
		cfg.ActivationTTL != 2*time.Hour || cfg.ActivationResendInterval != 30*time.Second ||
		cfg.ActivationMailTimeout != 5*time.Second ||
		cfg.DBHost != "db" || cfg.DBPassword != "fake-password" ||
		cfg.SMTPFrom != "sender@example.test" {
		t.Fatal("Overrides not applied")
	}
}

func TestLoadRejectsUnsafeConfigurationWithoutLeakingValues(t *testing.T) {
	for _, tc := range []struct{ key, value string }{
		{"JWT_ACCESS_SECRET", ""},
		{"JWT_REFRESH_SECRET", ""},
		{"JWT_ACCESS_SECRET", "short-test-secret"},
		{"JWT_REFRESH_SECRET", "short-test-secret"},
		{"JWT_ACCESS_SECRET", strings.Repeat(" ", 32)},
		{"JWT_REFRESH_SECRET", strings.Repeat("a", 32)},
		{"JWT_ACCESS_TTL", "private-invalid-value"},
		{"JWT_REFRESH_TTL", "private-invalid-value"},
		{"JWT_ACCESS_TTL", "0s"},
		{"JWT_REFRESH_TTL", "-1h"},
		{"JWT_ACCESS_TTL", "1ms"},
		{"JWT_REFRESH_TTL", "15m"},
		{"JWT_ACCESS_TTL", "721h"},
		{"SMTP_AUTH_ENABLED", "private-invalid-value"},
		{"ACTIVATION_TTL", "bad"},
		{"ACTIVATION_RESEND_INTERVAL", "0s"},
		{"ACTIVATION_MAIL_TIMEOUT", "-1s"},
		{"SMTP_TLS_REQUIRED", "private-invalid-value"},
	} {
		t.Run(tc.key+"_"+tc.value, func(t *testing.T) {
			configTestEnv(t)
			t.Setenv(tc.key, tc.value)
			_, err := Load()
			if err == nil {
				t.Fatal("Unsafe configuration accepted")
			}
			if !strings.Contains(err.Error(), tc.key) {
				t.Fatal("Error must identify the setting")
			}
			for _, secret := range []string{
				"short-test-secret", strings.Repeat("a", 32), strings.Repeat("r", 32),
				"private-invalid-value",
			} {
				if strings.Contains(err.Error(), secret) {
					t.Fatal("Configuration value leaked")
				}
			}
		})
	}
}
