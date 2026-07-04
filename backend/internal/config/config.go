package config

import (
	"log"
	"os"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	AppENV string
	ServerPort string
	DBHost string
	DBPort string
	DBUser string
	DBPassword string
	DBName string
	DBSSLMode  string
	JWTAccessSecret  string
	JWTRefreshSecret string
	JWTAccessTTL     time.Duration
	JWTRefreshTTL    time.Duration
}

func Load() Config {
	err := godotenv.Load()
	if err != nil {
		log.Println(".env file not found")
	}

	return Config{
		AppENV: os.Getenv("APP_ENV"),
		ServerPort: os.Getenv("SERVER_PORT"),
		DBHost: os.Getenv("DB_HOST"),
		DBPort: os.Getenv("DB_PORT"),
		DBUser: os.Getenv("DB_USER"),
		DBPassword: os.Getenv("DB_PASSWORD"),
		DBName: os.Getenv("DB_NAME"),
		DBSSLMode: os.Getenv("DB_SSLMODE"),
		JWTAccessSecret:  os.Getenv("JWT_ACCESS_SECRET"),
		JWTRefreshSecret: os.Getenv("JWT_REFRESH_SECRET"),
		JWTAccessTTL:  parseDuration(os.Getenv("JWT_ACCESS_TTL"), 15*time.Minute),
		JWTRefreshTTL: parseDuration(os.Getenv("JWT_REFRESH_TTL"), 30*24*time.Hour),
	}
}

func parseDuration(value string, fallback time.Duration) time.Duration {
	d, err := time.ParseDuration(value)

	if err != nil {
		return fallback
	}
	return d
}