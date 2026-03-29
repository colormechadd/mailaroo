-- migrate:up

CREATE TYPE outbound_status AS ENUM (
    'QUEUED',
    'SENDING',
    'DELIVERED',
    'DEFERRED',
    'FAILED'
);

CREATE TABLE outbound_job (
    id UUID PRIMARY KEY DEFAULT uuidv7(),
    email_id UUID REFERENCES email(id) ON DELETE SET NULL,
    from_address TEXT NOT NULL,
    recipients JSONB NOT NULL,
    raw_message BYTEA NOT NULL,
    status outbound_status NOT NULL DEFAULT 'QUEUED',
    attempt_count INT NOT NULL DEFAULT 0,
    max_attempts INT NOT NULL DEFAULT 5,
    last_error TEXT,
    next_attempt_datetime TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    delivery_datetime TIMESTAMPTZ,
    create_datetime TIMESTAMPTZ DEFAULT NOW(),
    update_datetime TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_outbound_job_status_next ON outbound_job(status, next_attempt_datetime)
    WHERE status IN ('QUEUED', 'DEFERRED');

-- migrate:down

DROP INDEX idx_outbound_job_status_next;
DROP TABLE outbound_job;
DROP TYPE outbound_status;
