-- migrate:up

ALTER TABLE public."user" ADD COLUMN recovery_email text DEFAULT '' NOT NULL;

-- migrate:down

ALTER TABLE public."user" DROP COLUMN recovery_email;
