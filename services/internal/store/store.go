package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	pgxdecimal "github.com/jackc/pgx-shopspring-decimal"
)

// DB wraps the connection pool and the generated queries.
type DB struct {
	Pool *pgxpool.Pool
	*Queries
}

// Open connects to PostgreSQL and registers the decimal type for NUMERIC columns.
func Open(ctx context.Context, url string) (*DB, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("parse database url: %w", err)
	}
	if cfg.MaxConns < 10 {
		cfg.MaxConns = 10
	}
	cfg.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		pgxdecimal.Register(conn.TypeMap())
		return nil
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("connect database: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}
	return &DB{Pool: pool, Queries: New(pool)}, nil
}

// Close releases the pool.
func (d *DB) Close() { d.Pool.Close() }

// Tx runs fn inside a transaction. The transaction commits when fn returns nil and rolls back otherwise.
func (d *DB) Tx(ctx context.Context, fn func(q *Queries, tx pgx.Tx) error) error {
	tx, err := d.Pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(d.Queries.WithTx(tx), tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// IsNotFound reports whether err means no row matched.
func IsNotFound(err error) bool { return errors.Is(err, pgx.ErrNoRows) }

// PgErrorCode returns the SQLSTATE of a PostgreSQL error, or "".
func PgErrorCode(err error) (string, string) {
	var pg *pgconn.PgError
	if errors.As(err, &pg) {
		return pg.Code, pg.ConstraintName
	}
	return "", ""
}
