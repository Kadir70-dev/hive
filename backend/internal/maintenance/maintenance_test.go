package maintenance_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"thehivesociety/api/internal/maintenance"
	"thehivesociety/api/internal/storage"
	"thehivesociety/api/internal/testdb"
)

func TestCleanupRemovesOnlyExpiredData(t *testing.T) {
	db := testdb.Open(t)
	u := testdb.User(t, db, "m@example.test", "approved", false)
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := db.Exec(q, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`INSERT INTO sessions(id_hash,user_id,expires_at) VALUES('live',$1,now()+interval '1 day'),('dead',$1,now()-interval '1 minute')`, u)
	exec(`INSERT INTO email_tokens(token_hash,user_id,purpose,expires_at) VALUES('t-new',$1,'verify_email',now()+interval '1 day'),('t-old',$1,'verify_email',now()-interval '8 days')`, u)
	exec(`INSERT INTO email_outbox(template,to_addr,subject,text_body,created_at) VALUES('x','a@example.test','s','secret link',now()-interval '3 days')`)
	exec(`INSERT INTO email_outbox(template,to_addr,subject,sent_at) VALUES('x','b@example.test','s',now()-interval '31 days')`)
	exec(`INSERT INTO email_outbox(template,to_addr,subject,text_body) VALUES('x','c@example.test','s','fresh')`)

	c, err := maintenance.Once(context.Background(), db, maintenance.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if c.Sessions != 1 || c.Tokens != 1 || c.OutboxExpired != 1 || c.OutboxDeleted != 1 {
		t.Fatalf("counts: %+v", c)
	}
	var live, fresh int
	_ = db.QueryRow(`SELECT count(*) FROM sessions WHERE id_hash='live'`).Scan(&live)
	_ = db.QueryRow(`SELECT count(*) FROM email_outbox WHERE to_addr='c@example.test' AND text_body='fresh'`).Scan(&fresh)
	if live != 1 || fresh != 1 {
		t.Fatal("live data removed")
	}
	var leftover *string
	_ = db.QueryRow(`SELECT text_body FROM email_outbox WHERE to_addr='a@example.test'`).Scan(&leftover)
	if leftover != nil {
		t.Fatal("stale undelivered body must be erased")
	}
}

func TestAuditRetentionBoundaries(t *testing.T) {
	db := testdb.Open(t)
	exec := func(q string) {
		t.Helper()
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	exec(`INSERT INTO audit_events(action,created_at) VALUES
  ('old', now() - interval '91 days'),
  ('edge-older', now() - interval '90 days' - interval '1 minute'),
  ('edge-newer', now() - interval '90 days' + interval '1 minute'),
  ('recent', now() - interval '1 day')`)
	ctx := context.Background()

	// Disabled by default: nothing is deleted.
	if c, err := maintenance.Once(ctx, db, maintenance.Options{}); err != nil || c.AuditDeleted != 0 {
		t.Fatalf("default must keep audit events: %+v %v", c, err)
	}
	// Below the safety minimum is refused, deleting nothing.
	if _, err := maintenance.Once(ctx, db, maintenance.Options{AuditRetention: 7 * 24 * time.Hour}); err == nil {
		t.Fatal("retention below minimum accepted")
	}
	c, err := maintenance.Once(ctx, db, maintenance.Options{AuditRetention: 90 * 24 * time.Hour})
	if err != nil || c.AuditDeleted != 2 {
		t.Fatalf("90-day retention: %+v %v", c, err)
	}
	rows, _ := db.Query(`SELECT action FROM audit_events ORDER BY action`)
	defer rows.Close()
	var kept []string
	for rows.Next() {
		var a string
		_ = rows.Scan(&a)
		kept = append(kept, a)
	}
	if len(kept) != 2 || kept[0] != "edge-newer" || kept[1] != "recent" {
		t.Fatalf("kept %v", kept)
	}
}

func TestStorageDeletionQueue(t *testing.T) {
	db := testdb.Open(t)
	ctx := context.Background()
	store := storage.NewMemory()
	_ = store.Put(ctx, "avatars/a.jpg", []byte("x"), "image/jpeg")
	_ = store.Put(ctx, "avatars/keep.jpg", []byte("x"), "image/jpeg")
	if _, err := db.Exec(`INSERT INTO storage_deletions(object_key) VALUES ('avatars/a.jpg'),('avatars/missing.jpg')`); err != nil {
		t.Fatal(err)
	}
	// Storage outage: rows stay queued with an attempt count and error.
	store.FailDelete = errors.New("r2 unavailable")
	c, err := maintenance.Once(ctx, db, maintenance.Options{Store: store})
	if err != nil || c.ObjectsDeleted != 0 || c.ObjectFailures == 0 {
		t.Fatalf("outage run: %+v %v", c, err)
	}
	var attempts int
	_ = db.QueryRow(`SELECT min(attempts) FROM storage_deletions`).Scan(&attempts)
	if attempts < 1 {
		t.Fatal("failed deletions must be retried later")
	}
	store.FailDelete = nil
	c, err = maintenance.Once(ctx, db, maintenance.Options{Store: store})
	if err != nil || c.ObjectsDeleted != 2 {
		t.Fatalf("recovery run: %+v %v", c, err)
	}
	if keys := store.Keys(); len(keys) != 1 || keys[0] != "avatars/keep.jpg" {
		t.Fatalf("remaining objects: %v", keys)
	}
	var left int
	_ = db.QueryRow(`SELECT count(*) FROM storage_deletions`).Scan(&left)
	if left != 0 {
		t.Fatalf("queue not drained: %d", left)
	}
}
