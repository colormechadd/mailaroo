-- migrate:up

CREATE TABLE thread (
    id UUID PRIMARY KEY DEFAULT uuidv7(),
    mailbox_id UUID NOT NULL REFERENCES mailbox(id) ON DELETE CASCADE,
    subject TEXT,
    create_datetime TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    update_datetime TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

ALTER TABLE email ADD COLUMN thread_id UUID REFERENCES thread(id) ON DELETE SET NULL;
ALTER TABLE email ADD COLUMN in_reply_to TEXT;
ALTER TABLE email ADD COLUMN "references" TEXT; -- Postgres reserved word

CREATE INDEX idx_email_thread_id ON email(thread_id);
CREATE INDEX idx_email_message_id ON email(message_id);
CREATE INDEX idx_thread_mailbox_id ON thread(mailbox_id);

-- migrate:down

ALTER TABLE email DROP COLUMN "references";
ALTER TABLE email DROP COLUMN in_reply_to;
ALTER TABLE email DROP COLUMN thread_id;
DROP TABLE thread;
