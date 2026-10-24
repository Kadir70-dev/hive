// Package maintenance purges expired security data on a schedule.
package maintenance

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	"thehivesociety/api/internal/storage"
)

// Counts reports what one run removed.
type Counts struct {
	Sessions, Tokens, OutboxDeleted, OutboxExpired int64
	AuditDeleted, ObjectsDeleted, ObjectFailures   int64
}

// Options configure optional cleanup steps.
type Options struct {
	// AuditRetention deletes audit events older than this. Zero disables
	// audit deletion entirely (the default until the retention period is
	// approved); see MinAuditRetention.
	AuditRetention time.Duration
	// Store, when set, is used to delete objects queued in storage_deletions.
	Store storage.Store
}

// MinAuditRetention is the shortest retention the API will accept, so a typo
// cannot wipe the security trail.
const MinAuditRetention = 30 * 24 * time.Hour

// maxObjectAttempts stops retrying an object deletion after this many failures.
const maxObjectAttempts = 20

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
func Once(ctx context.Context, db *sql.DB, opts Options) (Counts, error) {
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
	if opts.AuditRetention > 0 {
		if opts.AuditRetention < MinAuditRetention {
			return c, fmt.Errorf("audit retention %s is below the %s minimum", opts.AuditRetention, MinAuditRetention)
		}
		cutoff := fmt.Sprintf("%d seconds", int64(opts.AuditRetention.Seconds()))
		if c.AuditDeleted, err = drainArg(ctx, db, `DELETE FROM audit_events WHERE id IN (SELECT id FROM audit_events WHERE created_at < now() - $2::interval ORDER BY id LIMIT $1)`, cutoff); err != nil {
			return c, err
		}
	}
	if opts.Store != nil {
		if c.ObjectsDeleted, c.ObjectFailures, err = deleteObjects(ctx, db, opts.Store); err != nil {
			return c, err
		}
	}
	return c, nil
}

func drainArg(ctx context.Context, db *sql.DB, query string, arg any) (int64, error) {
	var total int64
	for {
		res, err := db.ExecContext(ctx, query, batch, arg)
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

// deleteObjects removes objects queued for deletion. Each key is claimed with
// SKIP LOCKED so several API instances never work on the same row.
func deleteObjects(ctx context.Context, db *sql.DB, store storage.Store) (deleted, failed int64, err error) {
	for {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return deleted, failed, err
		}
		var key string
		err = tx.QueryRowContext(ctx, `SELECT object_key FROM storage_deletions WHERE attempts < $1
ORDER BY created_at FOR UPDATE SKIP LOCKED LIMIT 1`, maxObjectAttempts).Scan(&key)
		if err == sql.ErrNoRows {
			tx.Rollback()
			return deleted, failed, nil
		}
		if err != nil {
			tx.Rollback()
			return deleted, failed, err
		}
		if derr := store.Delete(ctx, key); derr != nil {
			failed++
			msg := derr.Error()
			if len(msg) > 300 {
				msg = msg[:300]
			}
			_, err = tx.ExecContext(ctx, `UPDATE storage_deletions SET attempts=attempts+1, last_error=$2 WHERE object_key=$1`, key, msg)
		} else {
			deleted++
			_, err = tx.ExecContext(ctx, `DELETE FROM storage_deletions WHERE object_key=$1`, key)
		}
		if err == nil {
			err = tx.Commit()
		} else {
			tx.Rollback()
		}
		if err != nil {
			return deleted, failed, err
		}
		if failed > 0 && deleted == 0 && failed >= 10 {
			return deleted, failed, nil // storage looks down: try again next run
		}
	}
}

// Run executes Once immediately and then every interval until ctx ends.
func Run(ctx context.Context, db *sql.DB, log *slog.Logger, every time.Duration, opts Options) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		c, err := Once(ctx, db, opts)
		if err != nil && ctx.Err() == nil {
			log.Error("maintenance failed", "err", err)
		} else if c != (Counts{}) {
			log.Info("maintenance", "expired_sessions", c.Sessions, "old_tokens", c.Tokens, "outbox_deleted", c.OutboxDeleted,
				"outbox_expired", c.OutboxExpired, "audit_deleted", c.AuditDeleted, "objects_deleted", c.ObjectsDeleted, "object_failures", c.ObjectFailures)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}
