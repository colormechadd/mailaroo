-- migrate:up

CREATE TABLE ingestion (
    id UUID PRIMARY KEY DEFAULT uuidv7(),
    message_id TEXT,
    from_address TEXT,
    to_address TEXT,
    status TEXT NOT NULL, -- e.g., "processing", "accepted", "rejected", "failed"
    create_datetime TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    update_datetime TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE ingestion_step (
    id UUID PRIMARY KEY DEFAULT uuidv7(),
    ingestion_id UUID NOT NULL REFERENCES ingestion(id) ON DELETE CASCADE,
    step_name TEXT NOT NULL, -- e.g., "spf", "dkim", "spam", "virus", "storage", "database"
    status TEXT NOT NULL, -- e.g., "pass", "fail", "error", "skipped"
    details JSONB, -- Store validation results or error messages
    duration_ms INT,
    create_datetime TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

ALTER TABLE email ADD COLUMN ingestion_id UUID REFERENCES ingestion(id) ON DELETE SET NULL;
CREATE INDEX idx_email_ingestion_id ON email(ingestion_id);
CREATE INDEX idx_ingestion_step_ingestion_id ON ingestion_step(ingestion_id);

-- migrate:down

ALTER TABLE email DROP COLUMN ingestion_id;
DROP TABLE ingestion_step;
DROP TABLE ingestion;
