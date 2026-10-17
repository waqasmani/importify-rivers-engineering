// Package migrate applies the embedded goose migrations.
package migrate

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"

	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"github.com/waqasmani/importify-rivers-engineering/db"
)

func provider(conn *sql.DB) (*goose.Provider, error) {
	sub, err := fs.Sub(db.Migrations, "migrations")
	if err != nil {
		return nil, err
	}
	return goose.NewProvider(goose.DialectPostgres, conn, sub)
}

func open(url string) (*sql.DB, error) {
	conn, err := sql.Open("pgx", url)
	if err != nil {
		return nil, err
	}
	return conn, nil
}

var _ = stdlib.GetDefaultDriver

// Up applies all pending migrations and returns the resulting version.
func Up(ctx context.Context, url string) (int64, error) {
	conn, err := open(url)
	if err != nil {
		return 0, err
	}
	defer conn.Close()
	p, err := provider(conn)
	if err != nil {
		return 0, err
	}
	if _, err := p.Up(ctx); err != nil {
		return 0, fmt.Errorf("apply migrations: %w", err)
	}
	return p.GetDBVersion(ctx)
}

// Down rolls back the most recent migration.
func Down(ctx context.Context, url string) (int64, error) {
	conn, err := open(url)
	if err != nil {
		return 0, err
	}
	defer conn.Close()
	p, err := provider(conn)
	if err != nil {
		return 0, err
	}
	if _, err := p.Down(ctx); err != nil {
		return 0, err
	}
	return p.GetDBVersion(ctx)
}

// Reset rolls back every migration. Only used by tests and explicit operator commands.
func Reset(ctx context.Context, url string) error {
	conn, err := open(url)
	if err != nil {
		return err
	}
	defer conn.Close()
	p, err := provider(conn)
	if err != nil {
		return err
	}
	_, err = p.DownTo(ctx, 0)
	return err
}

// Status describes the applied and pending migrations.
type Status struct {
	Current int64 `json:"current"`
	Latest  int64 `json:"latest"`
	Pending int   `json:"pending"`
}

// GetStatus reports the migration state of the database.
func GetStatus(ctx context.Context, url string) (Status, error) {
	conn, err := open(url)
	if err != nil {
		return Status{}, err
	}
	defer conn.Close()
	p, err := provider(conn)
	if err != nil {
		return Status{}, err
	}
	cur, err := p.GetDBVersion(ctx)
	if err != nil {
		return Status{}, err
	}
	srcs := p.ListSources()
	st := Status{Current: cur}
	for _, s := range srcs {
		if s.Version > st.Latest {
			st.Latest = s.Version
		}
		if s.Version > cur {
			st.Pending++
		}
	}
	return st, nil
}
