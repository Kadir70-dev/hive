package mail

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"strconv"
	"time"
)

// Execer is satisfied by *sql.DB and *sql.Tx.
type Execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// Enqueue stores msg for delivery. Call it inside the transaction that creates
// whatever the email refers to (e.g. a token) so both commit or neither does.
func Enqueue(ctx context.Context, q Execer, userID, template string, msg Message) error {
	var uid any
	if userID != "" {
		uid = userID
	}
	_, err := q.ExecContext(ctx, `INSERT INTO email_outbox(user_id,template,to_addr,subject,text_body,html_body) VALUES($1,$2,$3,$4,$5,$6)`,
		uid, template, msg.To, msg.Subject, msg.Text, msg.HTML)
	return err
}

// Worker delivers queued email with retries and exponential backoff.
type Worker struct {
	DB          *sql.DB
	Sender      Sender
	Log         *slog.Logger
	Batch       int           // rows claimed per poll
	MaxAttempts int           // after this many failures a row is abandoned
	Lease       time.Duration // how long a claimed row is hidden from other workers
}

func (w *Worker) defaults() {
	if w.Batch <= 0 {
		w.Batch = 10
	}
	if w.MaxAttempts <= 0 {
		w.MaxAttempts = 8
	}
	if w.Lease <= 0 {
		w.Lease = 2 * time.Minute
	}
	if w.Log == nil {
		w.Log = slog.Default()
	}
}

// Backoff returns the wait before retry number attempt (1-based): 30s, 1m, 2m … capped at 1h.
func Backoff(attempt int) time.Duration {
	d := 30 * time.Second
	for i := 1; i < attempt && d < time.Hour; i++ {
		d *= 2
	}
	if d > time.Hour {
		d = time.Hour
	}
	return d
}

type claimed struct {
	id                    int64
	attempts              int
	to, subject, text, ht string
}

// ProcessOnce claims due rows (SKIP LOCKED, safe with several API instances),
// sends them and records the outcome. It returns how many were delivered.
func (w *Worker) ProcessOnce(ctx context.Context) (int, error) {
	w.defaults()
	rows, err := w.DB.QueryContext(ctx, `
UPDATE email_outbox SET attempts=attempts+1, locked_until=now()+$2::interval
WHERE id IN (
  SELECT id FROM email_outbox
  WHERE sent_at IS NULL AND failed_at IS NULL AND next_attempt_at<=now() AND (locked_until IS NULL OR locked_until<now())
  ORDER BY id LIMIT $1 FOR UPDATE SKIP LOCKED)
RETURNING id, attempts, to_addr, subject, COALESCE(text_body,''), COALESCE(html_body,'')`,
		w.Batch, fmt.Sprintf("%d seconds", int(w.Lease.Seconds())))
	if err != nil {
		return 0, err
	}
	var jobs []claimed
	for rows.Next() {
		var c claimed
		if err := rows.Scan(&c.id, &c.attempts, &c.to, &c.subject, &c.text, &c.ht); err != nil {
			rows.Close()
			return 0, err
		}
		jobs = append(jobs, c)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}
	sent := 0
	for _, c := range jobs {
		providerID, err := w.Sender.Send(ctx, Message{To: c.to, Subject: c.subject, Text: c.text, HTML: c.ht}, "hive-outbox-"+strconv.FormatInt(c.id, 10))
		switch {
		case err == nil:
			sent++
			_, err = w.DB.ExecContext(ctx, `UPDATE email_outbox SET sent_at=now(), provider_id=$2, text_body=NULL, html_body=NULL, locked_until=NULL, last_error=NULL WHERE id=$1`, c.id, providerID)
		case IsPermanent(err) || c.attempts >= w.MaxAttempts:
			w.Log.Error("email abandoned", "outbox_id", c.id, "attempts", c.attempts, "err", err)
			_, err = w.DB.ExecContext(ctx, `UPDATE email_outbox SET failed_at=now(), last_error=$2, text_body=NULL, html_body=NULL, locked_until=NULL WHERE id=$1`, c.id, truncate(err.Error()))
		default:
			w.Log.Warn("email delivery failed; will retry", "outbox_id", c.id, "attempts", c.attempts, "err", err)
			_, err = w.DB.ExecContext(ctx, `UPDATE email_outbox SET next_attempt_at=now()+$2::interval, last_error=$3, locked_until=NULL WHERE id=$1`,
				c.id, fmt.Sprintf("%d seconds", int(Backoff(c.attempts).Seconds())), truncate(err.Error()))
		}
		if err != nil {
			return sent, err
		}
	}
	return sent, nil
}

func truncate(s string) string {
	if len(s) > 500 {
		return s[:500]
	}
	return s
}

// Run polls until ctx is cancelled.
func (w *Worker) Run(ctx context.Context, every time.Duration) {
	w.defaults()
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		if _, err := w.ProcessOnce(ctx); err != nil && ctx.Err() == nil {
			w.Log.Error("email worker", "err", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}
