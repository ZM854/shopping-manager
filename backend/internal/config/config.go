package config

import (
	"fmt"
	"log"
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

	DBHost     string
	DBPort     string
	DBUser     string
	DBPassword string
	DBName     string
	DBSSLMode  string

	JWTAccessSecret  string
	JWTRefreshSecret string

	JWTAccessTTL  time.Duration
	JWTRefreshTTL time.Duration

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

		DBHost:     os.Getenv("DB_HOST"),
		DBPort:     os.Getenv("DB_PORT"),
		DBUser:     os.Getenv("DB_USER"),
		DBPassword: os.Getenv("DB_PASSWORD"),
		DBName:     os.Getenv("DB_NAME"),
		DBSSLMode:  os.Getenv("DB_SSLMODE"),

		JWTAccessSecret:  accessSecret,
		JWTRefreshSecret: refreshSecret,

		JWTAccessTTL:  accessTTL,
		JWTRefreshTTL: refreshTTL,

		SMTPHost:        os.Getenv("SMTP_HOST"),
		SMTPPort:        os.Getenv("SMTP_PORT"),
		SMTPUser:        os.Getenv("SMTP_USER"),
		SMTPPassword:    os.Getenv("SMTP_PASSWORD"),
		SMTPFrom:        os.Getenv("SMTP_FROM"),
		SMTPAuthEnabled: smtpAuth,
		SMTPTLSRequired: smtpTLS,
	}, nil
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
