-- migrate:up

CREATE TABLE contact (
    id           UUID PRIMARY KEY DEFAULT uuidv7(),
    user_id      UUID NOT NULL REFERENCES "user"(id) ON DELETE CASCADE,
    first_name   TEXT NOT NULL DEFAULT '',
    last_name    TEXT NOT NULL DEFAULT '',
    email        TEXT NOT NULL,
    phone        TEXT NOT NULL DEFAULT '',
    street       TEXT NOT NULL DEFAULT '',
    city         TEXT NOT NULL DEFAULT '',
    state        TEXT NOT NULL DEFAULT '',
    postal_code  TEXT NOT NULL DEFAULT '',
    country      TEXT NOT NULL DEFAULT '',
    notes        TEXT NOT NULL DEFAULT '',
    is_favorite  BOOLEAN NOT NULL DEFAULT FALSE,
    create_datetime TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    update_datetime TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (user_id, email)
);

CREATE INDEX idx_contact_user_id ON contact(user_id);
CREATE INDEX idx_contact_user_email ON contact(user_id, email);
CREATE INDEX idx_contact_user_name ON contact(user_id, last_name, first_name);

-- migrate:down

DROP INDEX idx_contact_user_name;
DROP INDEX idx_contact_user_email;
DROP INDEX idx_contact_user_id;
DROP TABLE contact;
