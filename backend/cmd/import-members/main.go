// Command import-members brings members from offline Supabase exports into
// PostgreSQL. See docs/operations/member-migration.md.
//
//	import-members plan   --dir EXPORT_DIR                     (dry run, the default)
//	import-members apply  --dir EXPORT_DIR --confirm-database hive
//	import-members invite [--apply --confirm-database hive] [--limit 50] [--app-url https://thehivesociety.ae]
//
// Reports (JSON on stdout) never contain names, emails or phone numbers.
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"thehivesociety/api/internal/importer"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "import-members:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: import-members plan|apply|invite [flags]")
	}
	cmd := args[0]
	fs := flag.NewFlagSet(cmd, flag.ContinueOnError)
	dir := fs.String("dir", "", "directory containing members.csv [gatherings.csv bookings.csv]")
	confirm := fs.String("confirm-database", "", "target database name (required to write)")
	apply := fs.Bool("apply", false, "invite: actually queue invitations")
	limit := fs.Int("limit", 50, "invite: maximum invitations in this run")
	appURL := fs.String("app-url", "https://thehivesociety.ae", "invite: site origin used in links")
	if err := fs.Parse(args[1:]); err != nil {
		return err
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
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	var report any
	switch cmd {
	case "plan", "apply":
		if *dir == "" {
			return fmt.Errorf("--dir is required")
		}
		in, err := importer.Load(*dir)
		if err != nil {
			return err
		}
		if cmd == "apply" {
			if err := importer.CheckDatabase(ctx, db, *confirm); err != nil {
				return err
			}
			report, err = importer.Apply(ctx, db, in)
		} else {
			report, err = importer.Plan(ctx, db, in)
		}
		if err != nil {
			return err
		}
	case "invite":
		if *limit < 1 || *limit > 1000 {
			return fmt.Errorf("--limit must be between 1 and 1000")
		}
		if *apply {
			if err := importer.CheckDatabase(ctx, db, *confirm); err != nil {
				return err
			}
		}
		if report, err = importer.Invite(ctx, db, *appURL, *limit, *apply); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unknown command %q", cmd)
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(report)
}
