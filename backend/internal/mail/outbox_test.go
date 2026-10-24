package mail_test

import (
	"context"
	"errors"
	"testing"

	"thehivesociety/api/internal/mail"
	"thehivesociety/api/internal/testdb"
)

func TestOutboxDeliveryRetryAndErasure(t *testing.T) {
	db := testdb.Open(t)
	ctx := context.Background()
	if err := mail.Enqueue(ctx, db, "", mail.TemplatePasswordChanged, mail.PasswordChanged("ok@example.test")); err != nil {
		t.Fatal(err)
	}
	rec := &mail.Recorder{}
	w := &mail.Worker{DB: db, Sender: rec, MaxAttempts: 3}
	n, err := w.ProcessOnce(ctx)
	if err != nil || n != 1 || len(rec.Messages()) != 1 {
		t.Fatalf("deliver: n=%d err=%v sent=%d", n, err, len(rec.Messages()))
	}
	var body *string
	_ = db.QueryRow(`SELECT text_body FROM email_outbox WHERE to_addr='ok@example.test'`).Scan(&body)
	if body != nil {
		t.Fatal("delivered bodies must be erased")
	}
	if n, _ := w.ProcessOnce(ctx); n != 0 {
		t.Fatal("delivered mail must not be sent twice")
	}

	// Transient failure: retried later, not immediately.
	_ = mail.Enqueue(ctx, db, "", mail.TemplatePasswordChanged, mail.PasswordChanged("retry@example.test"))
	rec.Fail = errors.New("provider timeout")
	if n, _ := w.ProcessOnce(ctx); n != 0 {
		t.Fatal("failed send counted as delivered")
	}
	var attempts int
	var due bool
	_ = db.QueryRow(`SELECT attempts, next_attempt_at > now() FROM email_outbox WHERE to_addr='retry@example.test'`).Scan(&attempts, &due)
	if attempts != 1 || !due {
		t.Fatalf("attempts=%d scheduled_later=%v", attempts, due)
	}
	// Make it due again and exhaust attempts.
	for i := 0; i < 3; i++ {
		_, _ = db.Exec(`UPDATE email_outbox SET next_attempt_at=now() WHERE to_addr='retry@example.test'`)
		_, _ = w.ProcessOnce(ctx)
	}
	var failed bool
	_ = db.QueryRow(`SELECT failed_at IS NOT NULL AND text_body IS NULL FROM email_outbox WHERE to_addr='retry@example.test'`).Scan(&failed)
	if !failed {
		t.Fatal("exhausted mail should be abandoned with body erased")
	}

	// Permanent failure is abandoned at once.
	_ = mail.Enqueue(ctx, db, "", mail.TemplatePasswordChanged, mail.PasswordChanged("bad@example.test"))
	rec.Fail = &mail.PermanentError{Err: errors.New("invalid recipient")}
	_, _ = w.ProcessOnce(ctx)
	_ = db.QueryRow(`SELECT failed_at IS NOT NULL FROM email_outbox WHERE to_addr='bad@example.test'`).Scan(&failed)
	if !failed {
		t.Fatal("permanent failure should be abandoned immediately")
	}
}

// Two workers polling together never send the same row twice.
func TestOutboxConcurrentWorkers(t *testing.T) {
	db := testdb.Open(t)
	ctx := context.Background()
	for i := 0; i < 40; i++ {
		_ = mail.Enqueue(ctx, db, "", mail.TemplatePasswordChanged, mail.PasswordChanged("c@example.test"))
	}
	rec := &mail.Recorder{}
	done := make(chan int, 4)
	for i := 0; i < 4; i++ {
		go func() {
			w := &mail.Worker{DB: db, Sender: rec, Batch: 5}
			total := 0
			for {
				n, err := w.ProcessOnce(ctx)
				if err != nil || n == 0 {
					break
				}
				total += n
			}
			done <- total
		}()
	}
	sum := 0
	for i := 0; i < 4; i++ {
		sum += <-done
	}
	if sum != 40 || len(rec.Messages()) != 40 {
		t.Fatalf("delivered %d (recorded %d), want exactly 40", sum, len(rec.Messages()))
	}
}
