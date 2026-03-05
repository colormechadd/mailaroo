-- migrate:up

CREATE TYPE email_direction AS ENUM ('INBOUND', 'OUTBOUND');
CREATE TYPE email_status AS ENUM ('QUARANTINED', 'DELETED', 'INBOX', 'ARCHIVED');

CREATE TABLE "user" (
    id UUID PRIMARY KEY DEFAULT uuidv7(),
    username TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    is_active BOOLEAN DEFAULT TRUE,
    create_datetime TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    update_datetime TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE mailbox (
    id UUID PRIMARY KEY DEFAULT uuidv7(),
    user_id UUID NOT NULL REFERENCES "user"(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    create_datetime TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    update_datetime TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE address_mapping (
    id UUID PRIMARY KEY DEFAULT uuidv7(),
    address_pattern TEXT NOT NULL,
    mailbox_id UUID NOT NULL REFERENCES mailbox(id) ON DELETE CASCADE,
    priority INT DEFAULT 0,
    is_active BOOLEAN DEFAULT TRUE,
    create_datetime TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    update_datetime TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE sending_address (
    id UUID PRIMARY KEY DEFAULT uuidv7(),
    user_id UUID NOT NULL REFERENCES "user"(id) ON DELETE CASCADE,
    mailbox_id UUID NOT NULL REFERENCES mailbox(id) ON DELETE CASCADE,
    address TEXT NOT NULL,
    is_active BOOLEAN DEFAULT TRUE,
    create_datetime TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(user_id, address)
);

CREATE TABLE thread (
    id UUID PRIMARY KEY DEFAULT uuidv7(),
    mailbox_id UUID NOT NULL REFERENCES mailbox(id) ON DELETE CASCADE,
    subject TEXT,
    create_datetime TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    update_datetime TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE ingestion (
    id UUID PRIMARY KEY DEFAULT uuidv7(),
    message_id TEXT,
    from_address TEXT,
    to_address TEXT,
    status TEXT NOT NULL,
    create_datetime TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    update_datetime TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE email (
    id UUID PRIMARY KEY DEFAULT uuidv7(),
    mailbox_id UUID NOT NULL REFERENCES mailbox(id) ON DELETE CASCADE,
    address_mapping_id UUID REFERENCES address_mapping(id) ON DELETE SET NULL,
    ingestion_id UUID REFERENCES ingestion(id) ON DELETE SET NULL,
    thread_id UUID REFERENCES thread(id) ON DELETE SET NULL,
    sending_address_id UUID REFERENCES sending_address(id) ON DELETE SET NULL,
    message_id TEXT NOT NULL,
    subject TEXT,
    from_address TEXT NOT NULL,
    to_address TEXT NOT NULL,
    reply_to_address TEXT,
    in_reply_to TEXT,
    "references" TEXT,
    storage_key TEXT NOT NULL,
    size BIGINT NOT NULL,
    receive_datetime TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    read_datetime TIMESTAMP WITH TIME ZONE,
    star_datetime TIMESTAMP WITH TIME ZONE,
    is_read BOOLEAN DEFAULT FALSE,
    is_star BOOLEAN DEFAULT FALSE,
    direction email_direction NOT NULL,
    status email_status NOT NULL DEFAULT 'INBOX',
    create_datetime TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    update_datetime TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE email_attachment (
    id UUID PRIMARY KEY DEFAULT uuidv7(),
    email_id UUID NOT NULL REFERENCES email(id) ON DELETE CASCADE,
    filename TEXT NOT NULL,
    content_type TEXT NOT NULL,
    size BIGINT NOT NULL,
    storage_key TEXT NOT NULL,
    create_datetime TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    update_datetime TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE ingestion_step (
    id UUID PRIMARY KEY DEFAULT uuidv7(),
    ingestion_id UUID NOT NULL REFERENCES ingestion(id) ON DELETE CASCADE,
    step_name TEXT NOT NULL,
    status TEXT NOT NULL,
    details JSONB,
    duration_ms INT,
    create_datetime TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE mailbox_block_rule (
    id UUID PRIMARY KEY DEFAULT uuidv7(),
    mailbox_id UUID NOT NULL REFERENCES mailbox(id) ON DELETE CASCADE,
    address_pattern TEXT NOT NULL,
    is_active BOOLEAN DEFAULT TRUE,
    create_datetime TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    update_datetime TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE webmail_session (
    id UUID PRIMARY KEY DEFAULT uuidv7(),
    user_id UUID NOT NULL REFERENCES "user"(id) ON DELETE CASCADE,
    token TEXT NOT NULL UNIQUE,
    remote_ip TEXT,
    user_agent TEXT,
    expires_datetime TIMESTAMP WITH TIME ZONE NOT NULL,
    create_datetime TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

-- Indexes
CREATE INDEX idx_email_mailbox_id ON email(mailbox_id);
CREATE INDEX idx_email_address_mapping_id ON email(address_mapping_id);
CREATE INDEX idx_email_receive_datetime ON email(receive_datetime DESC);
CREATE INDEX idx_email_thread_id ON email(thread_id);
CREATE INDEX idx_email_message_id ON email(message_id);
CREATE INDEX idx_email_ingestion_id ON email(ingestion_id);
CREATE INDEX idx_email_sending_address_id ON email(sending_address_id);
CREATE INDEX idx_email_status ON email(status);
CREATE INDEX idx_email_is_read ON email(is_read);
CREATE INDEX idx_email_is_star ON email(is_star);
CREATE INDEX idx_email_direction ON email(direction);

CREATE INDEX idx_address_mapping_pattern ON address_mapping(address_pattern);
CREATE INDEX idx_ingestion_step_ingestion_id ON ingestion_step(ingestion_id);
CREATE INDEX idx_mailbox_block_rule_mailbox_id ON mailbox_block_rule(mailbox_id);
CREATE INDEX idx_thread_mailbox_id ON thread(mailbox_id);
CREATE INDEX idx_webmail_session_token ON webmail_session(token);
CREATE INDEX idx_sending_address_user_id ON sending_address(user_id);

-- migrate:down

DROP TABLE webmail_session;
DROP TABLE mailbox_block_rule;
DROP TABLE ingestion_step;
DROP TABLE email_attachment;
DROP TABLE email;
DROP TYPE email_status;
DROP TYPE email_direction;
DROP TABLE ingestion;
DROP TABLE thread;
DROP TABLE sending_address;
DROP TABLE address_mapping;
DROP TABLE mailbox;
DROP TABLE "user";
