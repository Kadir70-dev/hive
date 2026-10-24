// Package maintenance purges expired security data on a schedule.
package maintenance

import (
	"context"
	"database/sql"
	"log/slog"
	"time"
)

// Counts reports what one run removed.
type Counts struct {
	Sessions, Tokens, OutboxDeleted, OutboxExpired int64
}

// batch deletes in chunks so a large backlog never holds long locks.
const batch = 5000

func drain(ctx context.Context, db *sql.DB, query string) (int64, error) {
	var total int64
	for {
		res, err := db.ExecContext(ctx, query, batch)
		if err != nil {
			return total, err
		}
		n, _ := res.RowsAffected()
		total += n
		if n < batch {
			return total, nil
		}
	}
}

// Once runs every cleanup step a single time.
func Once(ctx context.Context, db *sql.DB) (Counts, error) {
	var c Counts
	var err error
	if c.Sessions, err = drain(ctx, db, `DELETE FROM sessions WHERE ctid IN (SELECT ctid FROM sessions WHERE expires_at < now() LIMIT $1)`); err != nil {
		return c, err
	}
	// Keep used/expired tokens for a week for incident review, then drop them.
	if c.Tokens, err = drain(ctx, db, `DELETE FROM email_tokens WHERE ctid IN (SELECT ctid FROM email_tokens WHERE expires_at < now() - interval '7 days' LIMIT $1)`); err != nil {
		return c, err
	}
	// Undelivered mail older than two days carries only expired links: abandon it and erase the body.
	res, err := db.ExecContext(ctx, `UPDATE email_outbox SET failed_at=now(), last_error='expired before delivery', text_body=NULL, html_body=NULL
WHERE sent_at IS NULL AND failed_at IS NULL AND created_at < now() - interval '2 days'`)
	if err != nil {
		return c, err
	}
	c.OutboxExpired, _ = res.RowsAffected()
	if c.OutboxDeleted, err = drain(ctx, db, `DELETE FROM email_outbox WHERE ctid IN (SELECT ctid FROM email_outbox WHERE COALESCE(sent_at, failed_at) < now() - interval '30 days' LIMIT $1)`); err != nil {
		return c, err
	}
	return c, nil
}

// Run executes Once immediately and then every interval until ctx ends.
func Run(ctx context.Context, db *sql.DB, log *slog.Logger, every time.Duration) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		c, err := Once(ctx, db)
		if err != nil && ctx.Err() == nil {
			log.Error("maintenance failed", "err", err)
		} else if c != (Counts{}) {
			log.Info("maintenance", "expired_sessions", c.Sessions, "old_tokens", c.Tokens, "outbox_deleted", c.OutboxDeleted, "outbox_expired", c.OutboxExpired)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}
