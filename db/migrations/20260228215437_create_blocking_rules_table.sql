-- migrate:up

CREATE TABLE mailbox_block_rule (
    id UUID PRIMARY KEY DEFAULT uuidv7(),
    mailbox_id UUID NOT NULL REFERENCES mailbox(id) ON DELETE CASCADE,
    address_pattern TEXT NOT NULL, -- Regex rule
    is_active BOOLEAN DEFAULT TRUE,
    create_datetime TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    update_datetime TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_mailbox_block_rule_mailbox_id ON mailbox_block_rule(mailbox_id);

-- migrate:down

DROP TABLE mailbox_block_rule;
