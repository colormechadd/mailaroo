-- migrate:up

ALTER TABLE email ADD COLUMN sending_address_id UUID REFERENCES sending_address(id) ON DELETE SET NULL;
CREATE INDEX idx_email_sending_address_id ON email(sending_address_id);

-- migrate:down

DROP INDEX idx_email_sending_address_id;
ALTER TABLE email DROP COLUMN sending_address_id;
