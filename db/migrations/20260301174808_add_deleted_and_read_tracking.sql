-- migrate:up

ALTER TABLE email ADD COLUMN is_deleted BOOLEAN DEFAULT FALSE;
ALTER TABLE email ADD COLUMN read_datetime TIMESTAMP WITH TIME ZONE;
ALTER TABLE email ADD COLUMN star_datetime TIMESTAMP WITH TIME ZONE;

CREATE INDEX idx_email_is_deleted ON email(is_deleted);
CREATE INDEX idx_email_is_read ON email(is_read);
CREATE INDEX idx_email_is_star ON email(is_star);

-- migrate:down

ALTER TABLE email DROP COLUMN star_datetime;
ALTER TABLE email DROP COLUMN read_datetime;
ALTER TABLE email DROP COLUMN is_deleted;
