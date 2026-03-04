-- migrate:up
ALTER TABLE email ADD COLUMN is_quarantined BOOLEAN DEFAULT FALSE;
CREATE INDEX idx_email_is_quarantined ON email(is_quarantined) WHERE is_quarantined = TRUE;

-- migrate:down
DROP INDEX idx_email_is_quarantined;
ALTER TABLE email DROP COLUMN is_quarantined;
