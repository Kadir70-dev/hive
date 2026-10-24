// Package testdb creates throwaway, fully migrated PostgreSQL databases for
// integration tests. Tests are skipped unless TEST_DATABASE_URL points at a
// disposable server (never a production database).
package testdb

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"net/url"
	"os"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"thehivesociety/api/internal/migrate"
)

// URL returns TEST_DATABASE_URL or skips the test.
func URL(t testing.TB) string {
	t.Helper()
	u := os.Getenv("TEST_DATABASE_URL")
	if u == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping PostgreSQL integration test")
	}
	return u
}

// Empty creates a new empty database and returns its DSN. It is dropped when
// the test finishes.
func Empty(t testing.TB) string {
	t.Helper()
	base := URL(t)
	admin, err := sql.Open("pgx", base)
	if err != nil {
		t.Fatal(err)
	}
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	name := "hive_test_" + hex.EncodeToString(b)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if _, err := admin.ExecContext(ctx, "CREATE DATABASE "+name); err != nil {
		t.Fatalf("create test database: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_, _ = admin.ExecContext(ctx, "DROP DATABASE IF EXISTS "+name+" WITH (FORCE)")
		admin.Close()
	})
	u, err := url.Parse(base)
	if err != nil {
		t.Fatal(err)
	}
	u.Path = "/" + name
	return u.String()
}

// Open returns a connection to a fresh database migrated to the latest version.
func Open(t testing.TB) *sql.DB {
	t.Helper()
	db, err := sql.Open("pgx", Empty(t))
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(40)
	t.Cleanup(func() { db.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	if _, err := migrate.Up(ctx, db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

// User inserts a member and returns its id. The password hash is a dummy.
func User(t testing.TB, db *sql.DB, email, status string, admin bool) string {
	t.Helper()
	var id string
	err := db.QueryRow(`INSERT INTO users(email,password_hash,first_name,status,is_admin) VALUES($1,'x',$2,$3,$4) RETURNING id::text`,
		email, "Member", status, admin).Scan(&id)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

// Gathering inserts a gathering with the given capacity and returns its id.
func Gathering(t testing.TB, db *sql.DB, capacity int) string {
	t.Helper()
	var id string
	err := db.QueryRow(`INSERT INTO gatherings(title,starts_on,starts_at,place,capacity) VALUES('Majlis evening','2030-01-15','18:00','Abu Dhabi',$1) RETURNING id::text`, capacity).Scan(&id)
	if err != nil {
		t.Fatal(err)
	}
	return id
}
