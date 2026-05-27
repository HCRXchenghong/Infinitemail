ALTER TABLE mail_accounts
  ADD COLUMN IF NOT EXISTS mailbox_username text NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS mailbox_password_secret text NOT NULL DEFAULT '';
