-- migrate:up

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
    address_pattern TEXT NOT NULL, -- Regex rule
    mailbox_id UUID NOT NULL REFERENCES mailbox(id) ON DELETE CASCADE,
    priority INT DEFAULT 0,
    is_active BOOLEAN DEFAULT TRUE,
    create_datetime TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    update_datetime TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE email (
    id UUID PRIMARY KEY DEFAULT uuidv7(),
    mailbox_id UUID NOT NULL REFERENCES mailbox(id) ON DELETE CASCADE,
    address_mapping_id UUID REFERENCES address_mapping(id) ON DELETE SET NULL,
    message_id TEXT NOT NULL,
    subject TEXT,
    from_address TEXT NOT NULL,
    to_address TEXT NOT NULL,
    reply_to_address TEXT,
    storage_key TEXT NOT NULL,
    size BIGINT NOT NULL,
    receive_datetime TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    is_read BOOLEAN DEFAULT FALSE,
    is_star BOOLEAN DEFAULT FALSE,
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

CREATE INDEX idx_email_mailbox_id ON email(mailbox_id);
CREATE INDEX idx_email_address_mapping_id ON email(address_mapping_id);
CREATE INDEX idx_email_receive_datetime ON email(receive_datetime DESC);
CREATE INDEX idx_address_mapping_pattern ON address_mapping(address_pattern);

-- migrate:down

DROP TABLE email_attachment;
DROP TABLE email;
DROP TABLE address_mapping;
DROP TABLE mailbox;
DROP TABLE "user";
