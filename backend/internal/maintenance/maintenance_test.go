package maintenance_test

import (
	"context"
	"testing"

	"thehivesociety/api/internal/maintenance"
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

	c, err := maintenance.Once(context.Background(), db)
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
