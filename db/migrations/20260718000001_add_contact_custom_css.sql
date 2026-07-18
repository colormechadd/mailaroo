-- migrate:up

ALTER TABLE contact ADD COLUMN custom_css text DEFAULT '' NOT NULL;

-- migrate:down

ALTER TABLE contact DROP COLUMN custom_css;
