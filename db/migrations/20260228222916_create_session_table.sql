-- migrate:up

CREATE TABLE webmail_session (
    id UUID PRIMARY KEY DEFAULT uuidv7(),
    user_id UUID NOT NULL REFERENCES "user"(id) ON DELETE CASCADE,
    token TEXT NOT NULL UNIQUE,
    remote_ip TEXT,
    user_agent TEXT,
    expires_datetime TIMESTAMP WITH TIME ZONE NOT NULL,
    create_datetime TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_webmail_session_token ON webmail_session(token);

-- migrate:down

DROP TABLE webmail_session;
