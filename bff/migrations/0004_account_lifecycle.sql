ALTER TABLE mail_accounts
  ADD COLUMN IF NOT EXISTS status text NOT NULL DEFAULT 'active',
  ADD COLUMN IF NOT EXISTS disabled_at timestamptz,
  ADD COLUMN IF NOT EXISTS password_reset_at timestamptz;

CREATE INDEX IF NOT EXISTS idx_mail_accounts_status
  ON mail_accounts(status);
