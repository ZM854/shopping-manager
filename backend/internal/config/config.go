package config

import (
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

func Load() Config {
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

		JWTAccessSecret:  os.Getenv("JWT_ACCESS_SECRET"),
		JWTRefreshSecret: os.Getenv("JWT_REFRESH_SECRET"),

		JWTAccessTTL:  parseDuration(os.Getenv("JWT_ACCESS_TTL"), 15*time.Minute),
		JWTRefreshTTL: parseDuration(os.Getenv("JWT_REFRESH_TTL"), 30*24*time.Hour),

		SMTPHost:        os.Getenv("SMTP_HOST"),
		SMTPPort:        os.Getenv("SMTP_PORT"),
		SMTPUser:        os.Getenv("SMTP_USER"),
		SMTPPassword:    os.Getenv("SMTP_PASSWORD"),
		SMTPFrom:        os.Getenv("SMTP_FROM"),
		SMTPAuthEnabled: parseBool(os.Getenv("SMTP_AUTH_ENABLED"), true),
		SMTPTLSRequired: parseBool(os.Getenv("SMTP_TLS_REQUIRED"), true),
	}
}

func parseDuration(value string, fallback time.Duration) time.Duration {
	d, err := time.ParseDuration(value)

	if err != nil {
		return fallback
	}
	return d
}

func parseBool(value string, fallback bool) bool {
	if value == "" {
		return fallback
	}
	result, err := strconv.ParseBool(value)
	if err != nil {
		log.Fatalf("invalid boolean configuration value %q", value)
	}
	return result
}
