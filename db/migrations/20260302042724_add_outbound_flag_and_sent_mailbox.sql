-- migrate:up

ALTER TABLE email ADD COLUMN is_outbound BOOLEAN DEFAULT FALSE;
CREATE INDEX idx_email_is_outbound ON email(is_outbound);

-- migrate:down

DROP INDEX idx_email_is_outbound;
ALTER TABLE email DROP COLUMN is_outbound;
