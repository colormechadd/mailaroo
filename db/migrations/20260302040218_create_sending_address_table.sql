-- migrate:up

CREATE TABLE sending_address (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES "user"(id) ON DELETE CASCADE,
    address TEXT NOT NULL,
    is_active BOOLEAN DEFAULT TRUE,
    create_datetime TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(user_id, address)
);

CREATE INDEX idx_sending_address_user_id ON sending_address(user_id);

-- migrate:down

DROP TABLE sending_address;
