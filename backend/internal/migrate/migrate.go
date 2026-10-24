// Package migrate applies the embedded schema migrations with goose.
package migrate

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/lock"

	"thehivesociety/api/migrations"
)

func provider(db *sql.DB) (*goose.Provider, error) {
	// A Postgres advisory lock serialises concurrent migrators (e.g. two deploys).
	locker, err := lock.NewPostgresSessionLocker()
	if err != nil {
		return nil, err
	}
	return goose.NewProvider(goose.DialectPostgres, db, migrations.FS, goose.WithSessionLocker(locker))
}

// Up applies all pending migrations. Each migration runs in a transaction, so
// a failure leaves the database at the last fully applied version.
func Up(ctx context.Context, db *sql.DB) ([]string, error) {
	p, err := provider(db)
	if err != nil {
		return nil, err
	}
	results, err := p.Up(ctx)
	applied := make([]string, 0, len(results))
	for _, r := range results {
		applied = append(applied, fmt.Sprintf("%s (%s)", r.Source.Path, r.Duration))
	}
	return applied, err
}

// Status lists each known migration and whether it is applied.
func Status(ctx context.Context, db *sql.DB) ([]string, error) {
	p, err := provider(db)
	if err != nil {
		return nil, err
	}
	st, err := p.Status(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(st))
	for _, s := range st {
		out = append(out, fmt.Sprintf("%-8s %s", s.State, s.Source.Path))
	}
	return out, nil
}

// Pending reports whether any migration has not been applied yet.
func Pending(ctx context.Context, db *sql.DB) (bool, error) {
	p, err := provider(db)
	if err != nil {
		return false, err
	}
	return p.HasPending(ctx)
}
