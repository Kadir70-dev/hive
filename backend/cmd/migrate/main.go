// Command migrate applies The Hive Society database migrations.
//
//	migrate up      apply all pending migrations (safe to repeat)
//	migrate status  list migrations and their state
//
// There is deliberately no "down" or "reset" command: roll back by restoring
// a backup. The process exits non-zero on any failure so deploys stop.
package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"thehivesociety/api/internal/migrate"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "migrate:", err)
		os.Exit(1)
	}
}

func run() error {
	if len(os.Args) != 2 || (os.Args[1] != "up" && os.Args[1] != "status") {
		return fmt.Errorf("usage: migrate up|status")
	}
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		return fmt.Errorf("DATABASE_URL required")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return err
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("database unreachable: %w", err)
	}
	var lines []string
	if os.Args[1] == "up" {
		lines, err = migrate.Up(ctx, db)
		if err == nil && len(lines) == 0 {
			lines = []string{"no pending migrations"}
		}
	} else {
		lines, err = migrate.Status(ctx, db)
	}
	for _, l := range lines {
		fmt.Println(l)
	}
	return err
}
