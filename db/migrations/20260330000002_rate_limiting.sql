-- migrate:up

CREATE TABLE ip_block (
    id UUID PRIMARY KEY DEFAULT uuidv7(),
    ip_address INET NOT NULL,
    reason TEXT,
    blocked_until TIMESTAMP WITH TIME ZONE,
    is_permanent BOOLEAN NOT NULL DEFAULT FALSE,
    create_datetime TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_ip_block_ip ON ip_block(ip_address);
CREATE INDEX idx_ip_block_until ON ip_block(blocked_until) WHERE is_permanent = FALSE;

CREATE TABLE greylist_entry (
    id UUID PRIMARY KEY DEFAULT uuidv7(),
    ip_address INET NOT NULL,
    from_address TEXT NOT NULL,
    to_address TEXT NOT NULL,
    first_seen TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    last_seen TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    pass_count INT NOT NULL DEFAULT 0,
    UNIQUE (ip_address, from_address, to_address)
);
CREATE INDEX idx_greylist_lookup ON greylist_entry(ip_address, from_address, to_address);

-- migrate:down

DROP INDEX idx_greylist_lookup;
DROP TABLE greylist_entry;
DROP INDEX idx_ip_block_until;
DROP INDEX idx_ip_block_ip;
DROP TABLE ip_block;
