-- migrate:up
ALTER TABLE public.email
  ADD COLUMN category text DEFAULT '' NOT NULL;

-- migrate:down
ALTER TABLE public.email DROP COLUMN category;
