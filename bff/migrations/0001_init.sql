-- InfiniteMail BFF production schema.
-- Target: PostgreSQL 15+

CREATE EXTENSION IF NOT EXISTS pgcrypto;
CREATE EXTENSION IF NOT EXISTS citext;

CREATE TABLE IF NOT EXISTS schema_migrations (
  version text PRIMARY KEY,
  applied_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS bff_state_snapshots (
  id text PRIMARY KEY,
  payload jsonb NOT NULL,
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS admin_mail_config (
  id boolean PRIMARY KEY DEFAULT true,
  mailbox jsonb NOT NULL,
  auth jsonb NOT NULL,
  sms jsonb NOT NULL,
  updated_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT admin_mail_config_singleton CHECK (id)
);

CREATE TABLE IF NOT EXISTS mail_accounts (
  id text PRIMARY KEY,
  phone text NOT NULL UNIQUE,
  email citext NOT NULL UNIQUE,
  display_name text NOT NULL,
  password_hash text NOT NULL,
  source text NOT NULL DEFAULT 'invite',
  registered_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS auth_sessions (
  token_hash text PRIMARY KEY,
  account_id text NOT NULL REFERENCES mail_accounts(id) ON DELETE CASCADE,
  created_at timestamptz NOT NULL DEFAULT now(),
  expires_at timestamptz NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_auth_sessions_account_id ON auth_sessions(account_id);
CREATE INDEX IF NOT EXISTS idx_auth_sessions_expires_at ON auth_sessions(expires_at);

CREATE TABLE IF NOT EXISTS mailbox_invites (
  id text PRIMARY KEY,
  code text NOT NULL UNIQUE,
  email citext NOT NULL,
  mailbox_local_part text NOT NULL,
  phone text,
  note text NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL DEFAULT now(),
  expires_at timestamptz,
  used_at timestamptz,
  url text NOT NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_mailbox_invites_active_email
  ON mailbox_invites(email)
  WHERE used_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_mailbox_invites_phone ON mailbox_invites(phone);
CREATE INDEX IF NOT EXISTS idx_mailbox_invites_created_at ON mailbox_invites(created_at DESC);

CREATE TABLE IF NOT EXISTS sms_code_logs (
  id text PRIMARY KEY,
  phone text NOT NULL,
  code text NOT NULL,
  purpose text NOT NULL,
  provider text NOT NULL,
  status text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  expires_at timestamptz NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_sms_code_logs_phone_purpose_created
  ON sms_code_logs(phone, purpose, created_at DESC);

CREATE TABLE IF NOT EXISTS mail_settings (
  account_id text PRIMARY KEY REFERENCES mail_accounts(id) ON DELETE CASCADE,
  default_sender_name text NOT NULL,
  signature text NOT NULL DEFAULT '',
  auto_reply_enabled boolean NOT NULL DEFAULT false,
  auto_reply_message text NOT NULL DEFAULT '',
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS mail_messages (
  id text PRIMARY KEY,
  account_id text NOT NULL REFERENCES mail_accounts(id) ON DELETE CASCADE,
  folder text NOT NULL,
  previous_folder text NOT NULL DEFAULT 'inbox',
  sender text NOT NULL,
  sender_email citext NOT NULL,
  recipients jsonb NOT NULL DEFAULT '[]'::jsonb,
  subject text NOT NULL,
  snippet text NOT NULL DEFAULT '',
  content_html text NOT NULL DEFAULT '',
  avatar text NOT NULL DEFAULT '',
  role text NOT NULL DEFAULT '',
  tags jsonb NOT NULL DEFAULT '[]'::jsonb,
  attachments jsonb NOT NULL DEFAULT '[]'::jsonb,
  is_unread boolean NOT NULL DEFAULT false,
  is_starred boolean NOT NULL DEFAULT false,
  has_attachment boolean NOT NULL DEFAULT false,
  is_outgoing boolean NOT NULL DEFAULT false,
  source text NOT NULL DEFAULT 'mailbox',
  delivery_status text NOT NULL DEFAULT 'received',
  sort_at timestamptz NOT NULL DEFAULT now(),
  sent_at timestamptz,
  received_at timestamptz
);

CREATE INDEX IF NOT EXISTS idx_mail_messages_account_folder_sort
  ON mail_messages(account_id, folder, sort_at DESC);
CREATE INDEX IF NOT EXISTS idx_mail_messages_account_starred
  ON mail_messages(account_id, is_starred)
  WHERE is_starred = true;
