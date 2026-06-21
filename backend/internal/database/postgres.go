package database

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/ZM854/shopping-manager/backend/internal/config"
	"github.com/jackc/pgx/v5/pgxpool"
)


func NewPostgres(cfg config.Config, log *slog.Logger) (*pgxpool.Pool, error)  {
	log = log.With("component", "postgres")

	log.Info("initializing postgres connection pool")

	connString := fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=%s",
		cfg.DBUser,
		cfg.DBPassword,
		cfg.DBHost,
		cfg.DBPort,
		cfg.DBName,
		cfg.DBSSLMode,
	)

	poolConfig, err := pgxpool.ParseConfig(connString)
	if err != nil {
		log.Error("failed to parse postgres config", "error", err)
		return nil, fmt.Errorf("parse postgres config: %w", err)
	}

	poolConfig.MaxConns = 10
	poolConfig.MinConns = 2
	poolConfig.MaxConnLifetime = time.Hour
	
	log.Debug("postgres pool configured", 
		"max_conns", poolConfig.MaxConns, 
		"min_conns", poolConfig.MinConns,
	)

	pool, err := pgxpool.NewWithConfig(context.Background(), poolConfig)
	if err != nil {
		log.Error("failed to create postgres pool", "error", err)
		return nil, fmt.Errorf("create postgres pool: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5 * time.Second)
	defer cancel()

	log.Info("checking postgres connection")

	if err := pool.Ping(ctx); err != nil {
		log.Error("failed to ping postgres", "error", err)
		return nil, fmt.Errorf("ping postgres: %w", err)
	}

	log.Info("postgres connection established")

	return pool, nil
}