-- migrate:up
ALTER TABLE "user" ADD COLUMN is_admin boolean DEFAULT false NOT NULL;

-- migrate:down
ALTER TABLE "user" DROP COLUMN is_admin;
