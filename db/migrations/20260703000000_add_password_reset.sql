-- migrate:up

CREATE TABLE public.password_reset (
    id uuid DEFAULT uuidv7() NOT NULL,
    user_id uuid NOT NULL,
    token text NOT NULL,
    expires_datetime timestamp with time zone NOT NULL,
    create_datetime timestamp with time zone DEFAULT CURRENT_TIMESTAMP
);

ALTER TABLE ONLY public.password_reset
    ADD CONSTRAINT password_reset_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.password_reset
    ADD CONSTRAINT password_reset_token_key UNIQUE (token);

ALTER TABLE ONLY public.password_reset
    ADD CONSTRAINT password_reset_user_id_fkey FOREIGN KEY (user_id) REFERENCES public."user"(id) ON DELETE CASCADE;

CREATE INDEX idx_password_reset_token ON public.password_reset USING btree (token);

-- migrate:down

DROP TABLE public.password_reset;
