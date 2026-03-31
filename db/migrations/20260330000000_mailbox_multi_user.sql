-- migrate:up

CREATE TABLE mailbox_user (
    id UUID PRIMARY KEY DEFAULT uuidv7(),
    mailbox_id UUID NOT NULL REFERENCES mailbox(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES "user"(id) ON DELETE CASCADE,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    create_datetime TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    update_datetime TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_mailbox_user_mailbox_id ON mailbox_user(mailbox_id);
CREATE INDEX idx_mailbox_user_user_id ON mailbox_user(user_id);
CREATE UNIQUE INDEX idx_mailbox_user_active ON mailbox_user(mailbox_id, user_id) WHERE is_active = TRUE;

-- Migrate existing ownership data
INSERT INTO mailbox_user (mailbox_id, user_id)
SELECT id, user_id FROM mailbox;

ALTER TABLE mailbox DROP COLUMN user_id;

ALTER TABLE email ADD COLUMN user_id UUID REFERENCES "user"(id) ON DELETE SET NULL;
CREATE INDEX idx_email_user_id ON email(user_id);

-- migrate:down

DROP INDEX idx_email_user_id;
ALTER TABLE email DROP COLUMN user_id;
ALTER TABLE mailbox ADD COLUMN user_id UUID REFERENCES "user"(id) ON DELETE CASCADE;
UPDATE mailbox m SET user_id = mu.user_id
    FROM mailbox_user mu
    WHERE mu.mailbox_id = m.id AND mu.is_active = TRUE;
DROP TABLE mailbox_user;
