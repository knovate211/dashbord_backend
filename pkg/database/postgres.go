// Package database provides connection factories for PostgreSQL and Redis.
package database

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// NewPostgresPool creates and validates a pgxpool connection pool from a DSN.
// DSN format: postgres://user:password@host:port/dbname?sslmode=disable
func NewPostgresPool(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	return NewPostgresPoolWithMax(ctx, dsn, 20)
}

// NewPostgresPoolWithMax is NewPostgresPool with an explicit connection cap,
// for a process that shares the database's connection limit with the services
// and should take a smaller slice of it.
func NewPostgresPoolWithMax(ctx context.Context, dsn string, maxConns int32) (*pgxpool.Pool, error) {
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse postgres config: %w", err)
	}

	config.MaxConns = maxConns
	config.MinConns = 2

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("create postgres pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}

	return pool, nil
}
