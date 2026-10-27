-- +goose Up
-- Link outbox rows that were queued without a user (account-exists notices and
-- admin new-member alerts) to their recipient's account, so deleting that
-- account removes them through the existing ON DELETE CASCADE. Data-only and
-- idempotent; rows addressed to unknown recipients are left untouched.
UPDATE email_outbox o SET user_id = u.id
FROM users u
WHERE o.user_id IS NULL AND o.template IN ('account_exists', 'new_member_alert') AND lower(o.to_addr) = u.email;

-- +goose Down
-- Nothing to undo: links only point rows at their real recipient.
SELECT 1;
