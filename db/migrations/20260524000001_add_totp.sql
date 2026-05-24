-- migrate:up
ALTER TABLE public."user"
  ADD COLUMN totp_enabled boolean NOT NULL DEFAULT false,
  ADD COLUMN totp_secret text,
  ADD COLUMN totp_backup_codes text[] NOT NULL DEFAULT '{}';

CREATE TABLE public.totp_pending (
    id uuid DEFAULT uuidv7() NOT NULL,
    user_id uuid NOT NULL REFERENCES public."user"(id) ON DELETE CASCADE,
    token text NOT NULL,
    expires_datetime timestamp with time zone NOT NULL,
    create_datetime timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT totp_pending_pkey PRIMARY KEY (id),
    CONSTRAINT totp_pending_token_key UNIQUE (token)
);

-- migrate:down
drop table if exists public.totp_pending;
alter table public."uesr" drop column totp_backup_codes;
alter table public."uesr" drop column totp_secret;
alter table public."uesr" drop column totp_enabled;