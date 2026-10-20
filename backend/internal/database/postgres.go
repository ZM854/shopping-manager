package database

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/url"
	"strconv"
	"time"

	"github.com/ZM854/shopping-manager/backend/internal/config"
	"github.com/jackc/pgx/v5/pgxpool"
)

func poolConfig(cfg config.Config) (*pgxpool.Config, error) {
	port, err := strconv.Atoi(cfg.DBPort)
	if err != nil || port < 1 || port > 65535 || cfg.DBHost == "" || cfg.DBUser == "" || cfg.DBName == "" {
		return nil, errors.New("invalid postgres host, port, user or database configuration")
	}
	switch cfg.DBSSLMode {
	case "disable", "allow", "prefer", "require", "verify-ca", "verify-full":
	default:
		return nil, errors.New("invalid postgres SSL mode")
	}
	connection := &url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(cfg.DBUser, cfg.DBPassword),
		Host:   net.JoinHostPort(cfg.DBHost, cfg.DBPort),
		Path:   "/" + cfg.DBName,
	}
	query := url.Values{"sslmode": {cfg.DBSSLMode}}
	connection.RawQuery = query.Encode()
	parsed, err := pgxpool.ParseConfig(connection.String())
	if err != nil {
		return nil, errors.New("invalid postgres connection configuration")
	}
	parsed.MaxConns = 10
	parsed.MinConns = 2
	parsed.MaxConnLifetime = time.Hour
	parsed.ConnConfig.ConnectTimeout = 5 * time.Second
	return parsed, nil
}

type connectionPool interface {
	Ping(context.Context) error
	Close()
}

func checkConnection(ctx context.Context, pool connectionPool) error {
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return errors.New("postgres connection check failed")
	}
	return nil
}

func NewPostgres(cfg config.Config, log *slog.Logger) (*pgxpool.Pool, error) {
	log = log.With("component", "postgres")
	log.Info("initializing postgres connection pool")
	parsed, err := poolConfig(cfg)
	if err != nil {
		return nil, err
	}
	pool, err := pgxpool.NewWithConfig(context.Background(), parsed)
	if err != nil {
		return nil, errors.New("postgres pool creation failed")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := checkConnection(ctx, pool); err != nil {
		return nil, err
	}
	log.Info("postgres connection established")
	return pool, nil
}
