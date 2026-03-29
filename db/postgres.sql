\restrict dbmate

-- Dumped from database version 18.3
-- Dumped by pg_dump version 18.3

SET statement_timeout = 0;
SET lock_timeout = 0;
SET idle_in_transaction_session_timeout = 0;
SET transaction_timeout = 0;
SET client_encoding = 'UTF8';
SET standard_conforming_strings = on;
SELECT pg_catalog.set_config('search_path', '', false);
SET check_function_bodies = false;
SET xmloption = content;
SET client_min_messages = warning;
SET row_security = off;

--
-- Name: email_direction; Type: TYPE; Schema: public; Owner: -
--

CREATE TYPE public.email_direction AS ENUM (
    'INBOUND',
    'OUTBOUND'
);


--
-- Name: email_status; Type: TYPE; Schema: public; Owner: -
--

CREATE TYPE public.email_status AS ENUM (
    'QUARANTINED',
    'DELETED',
    'INBOX',
    'ARCHIVED'
);


SET default_tablespace = '';

SET default_table_access_method = heap;

--
-- Name: address_mapping; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.address_mapping (
    id uuid DEFAULT uuidv7() NOT NULL,
    address_pattern text NOT NULL,
    mailbox_id uuid NOT NULL,
    priority integer DEFAULT 0,
    is_active boolean DEFAULT true,
    create_datetime timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    update_datetime timestamp with time zone DEFAULT CURRENT_TIMESTAMP
);


--
-- Name: dkim_key; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.dkim_key (
    id uuid DEFAULT uuidv7() NOT NULL,
    domain text NOT NULL,
    selector text DEFAULT 'maileroo'::text NOT NULL,
    key_data bytea NOT NULL,
    is_active boolean DEFAULT true,
    create_datetime timestamp with time zone DEFAULT now(),
    update_datetime timestamp with time zone DEFAULT now()
);


--
-- Name: email; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.email (
    id uuid DEFAULT uuidv7() NOT NULL,
    mailbox_id uuid NOT NULL,
    address_mapping_id uuid,
    ingestion_id uuid,
    thread_id uuid,
    sending_address_id uuid,
    message_id text NOT NULL,
    subject text,
    from_address text NOT NULL,
    to_address text NOT NULL,
    reply_to_address text,
    in_reply_to text,
    "references" text,
    storage_key text NOT NULL,
    size bigint NOT NULL,
    receive_datetime timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    read_datetime timestamp with time zone,
    star_datetime timestamp with time zone,
    is_read boolean DEFAULT false,
    is_star boolean DEFAULT false,
    direction public.email_direction NOT NULL,
    status public.email_status DEFAULT 'INBOX'::public.email_status NOT NULL,
    create_datetime timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    update_datetime timestamp with time zone DEFAULT CURRENT_TIMESTAMP
);


--
-- Name: email_attachment; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.email_attachment (
    id uuid DEFAULT uuidv7() NOT NULL,
    email_id uuid NOT NULL,
    filename text NOT NULL,
    content_type text NOT NULL,
    size bigint NOT NULL,
    storage_key text NOT NULL,
    create_datetime timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    update_datetime timestamp with time zone DEFAULT CURRENT_TIMESTAMP
);


--
-- Name: ingestion; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.ingestion (
    id uuid DEFAULT uuidv7() NOT NULL,
    message_id text,
    from_address text,
    to_address text,
    status text NOT NULL,
    create_datetime timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    update_datetime timestamp with time zone DEFAULT CURRENT_TIMESTAMP
);


--
-- Name: ingestion_step; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.ingestion_step (
    id uuid DEFAULT uuidv7() NOT NULL,
    ingestion_id uuid NOT NULL,
    step_name text NOT NULL,
    status text NOT NULL,
    details jsonb,
    duration_ms integer,
    create_datetime timestamp with time zone DEFAULT CURRENT_TIMESTAMP
);


--
-- Name: mailbox; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.mailbox (
    id uuid DEFAULT uuidv7() NOT NULL,
    user_id uuid NOT NULL,
    name text NOT NULL,
    create_datetime timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    update_datetime timestamp with time zone DEFAULT CURRENT_TIMESTAMP
);


--
-- Name: mailbox_block_rule; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.mailbox_block_rule (
    id uuid DEFAULT uuidv7() NOT NULL,
    mailbox_id uuid NOT NULL,
    address_pattern text NOT NULL,
    is_active boolean DEFAULT true,
    create_datetime timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    update_datetime timestamp with time zone DEFAULT CURRENT_TIMESTAMP
);


--
-- Name: schema_migrations; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.schema_migrations (
    version character varying NOT NULL
);


--
-- Name: sending_address; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.sending_address (
    id uuid DEFAULT uuidv7() NOT NULL,
    user_id uuid NOT NULL,
    mailbox_id uuid NOT NULL,
    address text NOT NULL,
    is_active boolean DEFAULT true,
    create_datetime timestamp with time zone DEFAULT CURRENT_TIMESTAMP
);


--
-- Name: thread; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.thread (
    id uuid DEFAULT uuidv7() NOT NULL,
    mailbox_id uuid NOT NULL,
    subject text,
    create_datetime timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    update_datetime timestamp with time zone DEFAULT CURRENT_TIMESTAMP
);


--
-- Name: user; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public."user" (
    id uuid DEFAULT uuidv7() NOT NULL,
    username text NOT NULL,
    password_hash text NOT NULL,
    is_active boolean DEFAULT true,
    create_datetime timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    update_datetime timestamp with time zone DEFAULT CURRENT_TIMESTAMP
);


--
-- Name: webmail_session; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.webmail_session (
    id uuid DEFAULT uuidv7() NOT NULL,
    user_id uuid NOT NULL,
    token text NOT NULL,
    remote_ip text,
    user_agent text,
    expires_datetime timestamp with time zone NOT NULL,
    create_datetime timestamp with time zone DEFAULT CURRENT_TIMESTAMP
);


--
-- Name: address_mapping address_mapping_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.address_mapping
    ADD CONSTRAINT address_mapping_pkey PRIMARY KEY (id);


--
-- Name: dkim_key dkim_key_domain_selector_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.dkim_key
    ADD CONSTRAINT dkim_key_domain_selector_key UNIQUE (domain, selector);


--
-- Name: dkim_key dkim_key_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.dkim_key
    ADD CONSTRAINT dkim_key_pkey PRIMARY KEY (id);


--
-- Name: email_attachment email_attachment_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.email_attachment
    ADD CONSTRAINT email_attachment_pkey PRIMARY KEY (id);


--
-- Name: email email_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.email
    ADD CONSTRAINT email_pkey PRIMARY KEY (id);


--
-- Name: ingestion ingestion_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ingestion
    ADD CONSTRAINT ingestion_pkey PRIMARY KEY (id);


--
-- Name: ingestion_step ingestion_step_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ingestion_step
    ADD CONSTRAINT ingestion_step_pkey PRIMARY KEY (id);


--
-- Name: mailbox_block_rule mailbox_block_rule_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mailbox_block_rule
    ADD CONSTRAINT mailbox_block_rule_pkey PRIMARY KEY (id);


--
-- Name: mailbox mailbox_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mailbox
    ADD CONSTRAINT mailbox_pkey PRIMARY KEY (id);


--
-- Name: schema_migrations schema_migrations_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.schema_migrations
    ADD CONSTRAINT schema_migrations_pkey PRIMARY KEY (version);


--
-- Name: sending_address sending_address_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sending_address
    ADD CONSTRAINT sending_address_pkey PRIMARY KEY (id);


--
-- Name: sending_address sending_address_user_id_address_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sending_address
    ADD CONSTRAINT sending_address_user_id_address_key UNIQUE (user_id, address);


--
-- Name: thread thread_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.thread
    ADD CONSTRAINT thread_pkey PRIMARY KEY (id);


--
-- Name: user user_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public."user"
    ADD CONSTRAINT user_pkey PRIMARY KEY (id);


--
-- Name: user user_username_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public."user"
    ADD CONSTRAINT user_username_key UNIQUE (username);


--
-- Name: webmail_session webmail_session_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.webmail_session
    ADD CONSTRAINT webmail_session_pkey PRIMARY KEY (id);


--
-- Name: webmail_session webmail_session_token_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.webmail_session
    ADD CONSTRAINT webmail_session_token_key UNIQUE (token);


--
-- Name: idx_address_mapping_pattern; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_address_mapping_pattern ON public.address_mapping USING btree (address_pattern);


--
-- Name: idx_email_address_mapping_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_email_address_mapping_id ON public.email USING btree (address_mapping_id);


--
-- Name: idx_email_direction; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_email_direction ON public.email USING btree (direction);


--
-- Name: idx_email_ingestion_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_email_ingestion_id ON public.email USING btree (ingestion_id);


--
-- Name: idx_email_is_read; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_email_is_read ON public.email USING btree (is_read);


--
-- Name: idx_email_is_star; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_email_is_star ON public.email USING btree (is_star);


--
-- Name: idx_email_mailbox_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_email_mailbox_id ON public.email USING btree (mailbox_id);


--
-- Name: idx_email_message_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_email_message_id ON public.email USING btree (message_id);


--
-- Name: idx_email_receive_datetime; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_email_receive_datetime ON public.email USING btree (receive_datetime DESC);


--
-- Name: idx_email_sending_address_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_email_sending_address_id ON public.email USING btree (sending_address_id);


--
-- Name: idx_email_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_email_status ON public.email USING btree (status);


--
-- Name: idx_email_thread_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_email_thread_id ON public.email USING btree (thread_id);


--
-- Name: idx_ingestion_step_ingestion_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_ingestion_step_ingestion_id ON public.ingestion_step USING btree (ingestion_id);


--
-- Name: idx_mailbox_block_rule_mailbox_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_mailbox_block_rule_mailbox_id ON public.mailbox_block_rule USING btree (mailbox_id);


--
-- Name: idx_sending_address_user_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_sending_address_user_id ON public.sending_address USING btree (user_id);


--
-- Name: idx_thread_mailbox_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_thread_mailbox_id ON public.thread USING btree (mailbox_id);


--
-- Name: idx_webmail_session_token; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_webmail_session_token ON public.webmail_session USING btree (token);


--
-- Name: address_mapping address_mapping_mailbox_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.address_mapping
    ADD CONSTRAINT address_mapping_mailbox_id_fkey FOREIGN KEY (mailbox_id) REFERENCES public.mailbox(id) ON DELETE CASCADE;


--
-- Name: email email_address_mapping_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.email
    ADD CONSTRAINT email_address_mapping_id_fkey FOREIGN KEY (address_mapping_id) REFERENCES public.address_mapping(id) ON DELETE SET NULL;


--
-- Name: email_attachment email_attachment_email_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.email_attachment
    ADD CONSTRAINT email_attachment_email_id_fkey FOREIGN KEY (email_id) REFERENCES public.email(id) ON DELETE CASCADE;


--
-- Name: email email_ingestion_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.email
    ADD CONSTRAINT email_ingestion_id_fkey FOREIGN KEY (ingestion_id) REFERENCES public.ingestion(id) ON DELETE SET NULL;


--
-- Name: email email_mailbox_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.email
    ADD CONSTRAINT email_mailbox_id_fkey FOREIGN KEY (mailbox_id) REFERENCES public.mailbox(id) ON DELETE CASCADE;


--
-- Name: email email_sending_address_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.email
    ADD CONSTRAINT email_sending_address_id_fkey FOREIGN KEY (sending_address_id) REFERENCES public.sending_address(id) ON DELETE SET NULL;


--
-- Name: email email_thread_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.email
    ADD CONSTRAINT email_thread_id_fkey FOREIGN KEY (thread_id) REFERENCES public.thread(id) ON DELETE SET NULL;


--
-- Name: ingestion_step ingestion_step_ingestion_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.ingestion_step
    ADD CONSTRAINT ingestion_step_ingestion_id_fkey FOREIGN KEY (ingestion_id) REFERENCES public.ingestion(id) ON DELETE CASCADE;


--
-- Name: mailbox_block_rule mailbox_block_rule_mailbox_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mailbox_block_rule
    ADD CONSTRAINT mailbox_block_rule_mailbox_id_fkey FOREIGN KEY (mailbox_id) REFERENCES public.mailbox(id) ON DELETE CASCADE;


--
-- Name: mailbox mailbox_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.mailbox
    ADD CONSTRAINT mailbox_user_id_fkey FOREIGN KEY (user_id) REFERENCES public."user"(id) ON DELETE CASCADE;


--
-- Name: sending_address sending_address_mailbox_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sending_address
    ADD CONSTRAINT sending_address_mailbox_id_fkey FOREIGN KEY (mailbox_id) REFERENCES public.mailbox(id) ON DELETE CASCADE;


--
-- Name: sending_address sending_address_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sending_address
    ADD CONSTRAINT sending_address_user_id_fkey FOREIGN KEY (user_id) REFERENCES public."user"(id) ON DELETE CASCADE;


--
-- Name: thread thread_mailbox_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.thread
    ADD CONSTRAINT thread_mailbox_id_fkey FOREIGN KEY (mailbox_id) REFERENCES public.mailbox(id) ON DELETE CASCADE;


--
-- Name: webmail_session webmail_session_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.webmail_session
    ADD CONSTRAINT webmail_session_user_id_fkey FOREIGN KEY (user_id) REFERENCES public."user"(id) ON DELETE CASCADE;


--
-- PostgreSQL database dump complete
--

\unrestrict dbmate


--
-- Dbmate schema migrations
--

INSERT INTO public.schema_migrations (version) VALUES
    ('20260228000000'),
    ('20260329000000');
