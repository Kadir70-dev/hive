// Command admin is the operator-only admin bootstrap tool.
//
//	admin promote --email you@example.com   (must be in HIVE_ADMIN_EMAILS and verified)
//	admin demote --email you@example.com
//	admin reset-mfa --email you@example.com (lost authenticator; signs the account out)
//	admin list
//
// Run on the server: docker compose --env-file backend/.env run --rm --entrypoint admin api promote --email ...
package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"os"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"thehivesociety/api/internal/admintool"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "admin:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: admin promote|demote|reset-mfa --email ADDRESS | admin list")
	}
	cmd := args[0]
	fs := flag.NewFlagSet(cmd, flag.ContinueOnError)
	email := fs.String("email", "", "account email")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if cmd != "list" && *email == "" {
		return fmt.Errorf("--email is required")
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
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	switch cmd {
	case "promote":
		err = admintool.Promote(ctx, db, admintool.Allowlist(os.Getenv("HIVE_ADMIN_EMAILS")), *email)
	case "demote":
		err = admintool.Demote(ctx, db, *email)
	case "reset-mfa":
		err = admintool.ResetMFA(ctx, db, *email)
	case "list":
		var lines []string
		if lines, err = admintool.List(ctx, db); err == nil {
			for _, l := range lines {
				fmt.Println(l)
			}
		}
		return err
	default:
		return fmt.Errorf("unknown command %q", cmd)
	}
	if err == nil {
		fmt.Printf("%s: done\n", cmd)
	}
	return err
}
