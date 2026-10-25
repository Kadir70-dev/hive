package migrate_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"thehivesociety/api/internal/migrate"
	"thehivesociety/api/internal/testdb"
	"thehivesociety/api/migrations"
)

// This is a real upgrade of a populated PostgreSQL database, not a fresh
// schema assertion. It must preserve existing users, reservations and photos.
func TestUpgradePopulatedV3ToLatest(t *testing.T) {
	dsn := testdb.Empty(t)
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	provider, err := goose.NewProvider(goose.DialectPostgres, db, migrations.FS)
	if err != nil {
		t.Fatal(err)
	}
	applied, err := provider.UpTo(ctx, 3)
	if err != nil {
		t.Fatalf("apply migrations through v3: %v", err)
	}
	if len(applied) != 3 {
		t.Fatalf("expected 3 initial migrations, applied %d", len(applied))
	}

	var uid, gid string
	if err := db.QueryRowContext(ctx, `INSERT INTO users(email,password_hash,first_name,status,avatar_path)
VALUES('legacy-member@example.test','test-only-hash','Member','approved','avatars/legacy-photo.jpg') RETURNING id::text`).Scan(&uid); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `INSERT INTO gatherings(title,starts_on,starts_at,place,capacity)
VALUES('Existing gathering','2031-08-15','18:00','Abu Dhabi',2) RETURNING id::text`).Scan(&gid); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO reservations(user_id,gathering_id,status) VALUES($1,$2,'confirmed')`, uid, gid); err != nil {
		t.Fatal(err)
	}
	if pending, err := migrate.Pending(ctx, db); err != nil || !pending {
		t.Fatalf("later migrations must still be pending: pending=%v err=%v", pending, err)
	}

	upgraded, err := migrate.Up(ctx, db)
	if err != nil {
		t.Fatalf("upgrade v3 -> latest: %v", err)
	}
	if len(upgraded) != 2 {
		t.Fatalf("expected v4 and v5 migrations, got %v", upgraded)
	}
	var email, avatar, status string
	if err := db.QueryRowContext(ctx, `SELECT u.email,u.avatar_path,r.status
FROM users u JOIN reservations r ON r.user_id=u.id AND r.gathering_id=$2 WHERE u.id=$1`, uid, gid).
		Scan(&email, &avatar, &status); err != nil {
		t.Fatal(err)
	}
	if email != "legacy-member@example.test" || avatar != "avatars/legacy-photo.jpg" || status != "confirmed" {
		t.Fatalf("upgrade corrupted existing data")
	}
	var hasCleanupTable, hasLegacyMap, hasConflictTable, hasAttributionTable bool
	if err := db.QueryRowContext(ctx, `SELECT to_regclass('storage_deletions') IS NOT NULL,
to_regclass('legacy_member_map') IS NOT NULL,
to_regclass('safety_conflict_decisions') IS NOT NULL,
to_regclass('signup_attribution') IS NOT NULL`).
		Scan(&hasCleanupTable, &hasLegacyMap, &hasConflictTable, &hasAttributionTable); err != nil {
		t.Fatal(err)
	}
	if !hasCleanupTable || !hasLegacyMap || !hasConflictTable || !hasAttributionTable {
		t.Fatal("v4/v5 support tables missing after upgrade")
	}
	again, err := migrate.Up(ctx, db)
	if err != nil || len(again) != 0 {
		t.Fatalf("re-running migrations must be a no-op: %v %v", again, err)
	}
	if pending, err := migrate.Pending(ctx, db); err != nil || pending {
		t.Fatalf("migration still pending: %v %v", pending, err)
	}
}
