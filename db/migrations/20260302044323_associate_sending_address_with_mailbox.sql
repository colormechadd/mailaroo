-- migrate:up

-- 1. Add mailbox_id to sending_address
ALTER TABLE sending_address ADD COLUMN mailbox_id UUID REFERENCES mailbox(id) ON DELETE CASCADE;

-- 2. Attempt to link existing sending addresses to an 'Inbox' or first available mailbox
UPDATE sending_address sa
SET mailbox_id = (
    SELECT id FROM mailbox m 
    WHERE m.user_id = sa.user_id 
    ORDER BY (CASE WHEN name ILIKE 'inbox' THEN 0 ELSE 1 END), create_datetime ASC 
    LIMIT 1
);

-- 3. Make it NOT NULL after population (if you have existing data, ensure every user has at least one mailbox first)
ALTER TABLE sending_address ALTER COLUMN mailbox_id SET NOT NULL;

-- 4. Clean up 'Sent' and 'Trash' system mailboxes (Optional, but keeps things clean)
-- Move emails from 'Sent' mailbox to the linked mailbox of the sender
UPDATE email e
SET mailbox_id = sa.mailbox_id
FROM sending_address sa
WHERE e.sending_address_id = sa.id AND e.mailbox_id IN (SELECT id FROM mailbox WHERE name IN ('Sent', 'Trash'));

DELETE FROM mailbox WHERE name IN ('Sent', 'Trash');

-- migrate:down

ALTER TABLE sending_address DROP COLUMN mailbox_id;
