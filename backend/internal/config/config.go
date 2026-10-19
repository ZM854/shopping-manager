package config

import (
	"fmt"
	"log"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	AppENV       string
	ServerPort   string
	PublicAPIURL string
	FrontendURL  string
	CookieSecure bool

	DBHost     string
	DBPort     string
	DBUser     string
	DBPassword string
	DBName     string
	DBSSLMode  string

	JWTAccessSecret  string
	JWTRefreshSecret string

	JWTAccessTTL             time.Duration
	JWTRefreshTTL            time.Duration
	ActivationTTL            time.Duration
	ActivationResendInterval time.Duration
	ActivationMailTimeout    time.Duration

	SMTPHost        string
	SMTPPort        string
	SMTPUser        string
	SMTPPassword    string
	SMTPFrom        string
	SMTPAuthEnabled bool
	SMTPTLSRequired bool
}

func Load() (Config, error) {
	err := godotenv.Load()
	if err != nil {
		log.Println(".env file not found")
	}

	serverPort := os.Getenv("SERVER_PORT")
	publicAPIURL := os.Getenv("PUBLIC_API_URL")
	if publicAPIURL == "" {
		publicAPIURL = "http://localhost" + serverPort + "/api"
	}
	frontendURL := os.Getenv("FRONTEND_URL")
	if frontendURL == "" {
		frontendURL = "http://localhost:5173"
	}
	cookieSecure, err := validateSessionURLs(os.Getenv("APP_ENV"), publicAPIURL, frontendURL)
	if err != nil {
		return Config{}, err
	}
	accessTTL, err := parseDuration("JWT_ACCESS_TTL", os.Getenv("JWT_ACCESS_TTL"), 15*time.Minute)
	if err != nil {
		return Config{}, err
	}
	refreshTTL, err := parseDuration("JWT_REFRESH_TTL", os.Getenv("JWT_REFRESH_TTL"), 30*24*time.Hour)
	if err != nil {
		return Config{}, err
	}
	if refreshTTL <= accessTTL {
		return Config{}, fmt.Errorf("JWT_REFRESH_TTL must exceed JWT_ACCESS_TTL")
	}
	accessSecret := os.Getenv("JWT_ACCESS_SECRET")
	refreshSecret := os.Getenv("JWT_REFRESH_SECRET")
	for _, secret := range []struct{ name, value string }{
		{"JWT_ACCESS_SECRET", accessSecret},
		{"JWT_REFRESH_SECRET", refreshSecret},
	} {
		if len(secret.value) < 32 || strings.TrimSpace(secret.value) == "" {
			return Config{}, fmt.Errorf("%s must contain at least 32 bytes and must not be blank", secret.name)
		}
	}
	if accessSecret == refreshSecret {
		return Config{}, fmt.Errorf("JWT_ACCESS_SECRET and JWT_REFRESH_SECRET must differ")
	}
	activationTTL, err := parseDuration("ACTIVATION_TTL", os.Getenv("ACTIVATION_TTL"), 24*time.Hour)
	if err != nil {
		return Config{}, err
	}
	activationResend, err := parseDuration("ACTIVATION_RESEND_INTERVAL", os.Getenv("ACTIVATION_RESEND_INTERVAL"), time.Minute)
	if err != nil {
		return Config{}, err
	}
	activationTimeout, err := parseDuration("ACTIVATION_MAIL_TIMEOUT", os.Getenv("ACTIVATION_MAIL_TIMEOUT"), 10*time.Second)
	if err != nil {
		return Config{}, err
	}
	smtpAuth, err := parseBool("SMTP_AUTH_ENABLED", os.Getenv("SMTP_AUTH_ENABLED"), true)
	if err != nil {
		return Config{}, err
	}
	smtpTLS, err := parseBool("SMTP_TLS_REQUIRED", os.Getenv("SMTP_TLS_REQUIRED"), true)
	if err != nil {
		return Config{}, err
	}
	return Config{
		AppENV:       os.Getenv("APP_ENV"),
		ServerPort:   serverPort,
		PublicAPIURL: strings.TrimRight(publicAPIURL, "/"),
		FrontendURL:  strings.TrimRight(frontendURL, "/"),
		CookieSecure: cookieSecure,

		DBHost:     os.Getenv("DB_HOST"),
		DBPort:     os.Getenv("DB_PORT"),
		DBUser:     os.Getenv("DB_USER"),
		DBPassword: os.Getenv("DB_PASSWORD"),
		DBName:     os.Getenv("DB_NAME"),
		DBSSLMode:  os.Getenv("DB_SSLMODE"),

		JWTAccessSecret:  accessSecret,
		JWTRefreshSecret: refreshSecret,

		JWTAccessTTL:             accessTTL,
		JWTRefreshTTL:            refreshTTL,
		ActivationTTL:            activationTTL,
		ActivationResendInterval: activationResend,
		ActivationMailTimeout:    activationTimeout,

		SMTPHost:        os.Getenv("SMTP_HOST"),
		SMTPPort:        os.Getenv("SMTP_PORT"),
		SMTPUser:        os.Getenv("SMTP_USER"),
		SMTPPassword:    os.Getenv("SMTP_PASSWORD"),
		SMTPFrom:        os.Getenv("SMTP_FROM"),
		SMTPAuthEnabled: smtpAuth,
		SMTPTLSRequired: smtpTLS,
	}, nil
}

func validateSessionURLs(appEnv, apiURL, frontendURL string) (bool, error) {
	var secure bool
	for index, entry := range []struct{ name, value string }{
		{"PUBLIC_API_URL", apiURL}, {"FRONTEND_URL", frontendURL},
	} {
		u, err := url.Parse(entry.value)
		if err != nil || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" ||
			(u.Scheme != "https" && u.Scheme != "http") {
			return false, fmt.Errorf("%s must be an absolute HTTP(S) URL without credentials, query or fragment", entry.name)
		}
		if u.Scheme == "http" && appEnv != "dev" {
			return false, fmt.Errorf("%s requires HTTPS outside APP_ENV=dev", entry.name)
		}
		if index == 0 {
			secure = u.Scheme == "https"
		} else if secure != (u.Scheme == "https") {
			return false, fmt.Errorf("PUBLIC_API_URL and FRONTEND_URL must use the same scheme")
		}
	}
	return secure, nil
}

func parseDuration(name string, value string, fallback time.Duration) (time.Duration, error) {
	if value == "" {
		return fallback, nil
	}
	d, err := time.ParseDuration(value)
	if err != nil || d < time.Second {
		return 0, fmt.Errorf("%s must be a duration of at least 1s", name)
	}
	return d, nil
}

func parseBool(name string, value string, fallback bool) (bool, error) {
	if value == "" {
		return fallback, nil
	}
	result, err := strconv.ParseBool(value)
	if err != nil {
		return false, fmt.Errorf("%s must be a boolean", name)
	}
	return result, nil
}
