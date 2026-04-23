package database

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gitwise-io/gitwise/internal/config"
)

// poolConfig holds resolved connection-pool settings. Exported only for testing.
type poolConfig struct {
	MaxConns        int32
	MinConns        int32
	MaxConnLifetime time.Duration
	MaxConnIdleTime time.Duration
}

// poolConfigFromEnv builds pool settings from environment variables, falling
// back to the provided defaults when an env var is absent or invalid.
//
//   GITWISE_DB_MAX_CONNS  — maximum open connections (default 50)
//   GITWISE_DB_MIN_CONNS  — minimum idle connections  (default 10)
func poolConfigFromEnv(defaultMax, defaultMin int32) poolConfig {
	return poolConfig{
		MaxConns:        envInt32("GITWISE_DB_MAX_CONNS", defaultMax),
		MinConns:        envInt32("GITWISE_DB_MIN_CONNS", defaultMin),
		MaxConnLifetime: 1 * time.Hour,
		MaxConnIdleTime: 30 * time.Minute,
	}
}

func envInt32(key string, fallback int32) int32 {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.ParseInt(v, 10, 32); err == nil && n > 0 {
			return int32(n)
		}
	}
	return fallback
}

func Connect(ctx context.Context, cfg config.DatabaseConfig) (*pgxpool.Pool, error) {
	poolCfg, err := pgxpool.ParseConfig(cfg.DSN())
	if err != nil {
		return nil, fmt.Errorf("parse database config: %w", err)
	}

	pc := poolConfigFromEnv(50, 10)
	poolCfg.MaxConns = pc.MaxConns
	poolCfg.MinConns = pc.MinConns
	poolCfg.MaxConnLifetime = pc.MaxConnLifetime
	poolCfg.MaxConnIdleTime = pc.MaxConnIdleTime

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("create connection pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}

	return pool, nil
}
