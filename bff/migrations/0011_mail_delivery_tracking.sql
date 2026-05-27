ALTER TABLE mail_messages
  ADD COLUMN IF NOT EXISTS accepted_at timestamptz,
  ADD COLUMN IF NOT EXISTS provider_message_id text NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS delivery_error text NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_mail_messages_provider_message_id
  ON mail_messages(provider_message_id)
  WHERE provider_message_id <> '';
