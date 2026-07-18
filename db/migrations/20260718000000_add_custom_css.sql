-- migrate:up

ALTER TABLE "user" ADD COLUMN custom_css text DEFAULT '' NOT NULL;

-- migrate:down

ALTER TABLE "user" DROP COLUMN custom_css;
