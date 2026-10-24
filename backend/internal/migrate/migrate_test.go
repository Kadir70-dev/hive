package migrate_test

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"thehivesociety/api/internal/migrate"
	"thehivesociety/api/internal/testdb"
)

func open(t *testing.T) *sql.DB {
	db, err := sql.Open("pgx", testdb.Empty(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func ctx(t *testing.T) context.Context {
	c, cancel := context.WithTimeout(context.Background(), time.Minute)
	t.Cleanup(cancel)
	return c
}

func TestFreshDatabaseAndRepeatRun(t *testing.T) {
	db := open(t)
	if pending, err := migrate.Pending(ctx(t), db); err != nil || !pending {
		t.Fatalf("fresh database should have pending migrations: %v %v", pending, err)
	}
	applied, err := migrate.Up(ctx(t), db)
	if err != nil || len(applied) == 0 {
		t.Fatalf("first run: %v %v", applied, err)
	}
	again, err := migrate.Up(ctx(t), db)
	if err != nil || len(again) != 0 {
		t.Fatalf("second run must be a no-op: %v %v", again, err)
	}
	if pending, err := migrate.Pending(ctx(t), db); err != nil || pending {
		t.Fatalf("no migrations should be pending: %v %v", pending, err)
	}
	for _, table := range []string{"users", "sessions", "gatherings", "reservations", "attendance", "announcements", "partners", "member_reports", "member_blocks", "founding_signups", "audit_events"} {
		var ok bool
		if err := db.QueryRow(`SELECT to_regclass($1) IS NOT NULL`, "public."+table).Scan(&ok); err != nil || !ok {
			t.Errorf("table %s missing", table)
		}
	}
}

// Databases initialised by hand from the legacy 001_init.sql keep their data.
func TestUpgradeFromLegacyManualSchema(t *testing.T) {
	db := open(t)
	legacy, err := os.ReadFile("../../migrations/testdata/legacy_001_init.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx(t), string(legacy)); err != nil {
		t.Fatalf("legacy schema: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO users(email,password_hash,first_name) VALUES('legacy@example.test','x','Legacy')`); err != nil {
		t.Fatal(err)
	}
	if _, err := migrate.Up(ctx(t), db); err != nil {
		t.Fatalf("upgrade: %v", err)
	}
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM users WHERE email='legacy@example.test'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("legacy data lost: %d %v", n, err)
	}
}

func TestStatus(t *testing.T) {
	db := open(t)
	if _, err := migrate.Up(ctx(t), db); err != nil {
		t.Fatal(err)
	}
	lines, err := migrate.Status(ctx(t), db)
	if err != nil || len(lines) == 0 {
		t.Fatalf("status: %v %v", lines, err)
	}
}
