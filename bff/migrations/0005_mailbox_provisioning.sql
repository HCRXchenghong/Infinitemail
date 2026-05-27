ALTER TABLE mail_accounts
  ADD COLUMN IF NOT EXISTS mailbox_status text NOT NULL DEFAULT 'pending_config',
  ADD COLUMN IF NOT EXISTS mailbox_provisioned_at timestamptz,
  ADD COLUMN IF NOT EXISTS mailbox_external_id text NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS mailbox_last_error text NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_mail_accounts_mailbox_status
  ON mail_accounts(mailbox_status);
