ALTER TABLE auth_sessions
  ADD COLUMN IF NOT EXISTS session_id text,
  ADD COLUMN IF NOT EXISTS ip text NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS user_agent text NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS device_label text NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS last_seen_at timestamptz NOT NULL DEFAULT now();

UPDATE auth_sessions
SET session_id = 'sess_' || substring(encode(digest(token_hash, 'sha256'), 'hex') from 1 for 16)
WHERE session_id IS NULL OR session_id = '';

ALTER TABLE auth_sessions
  ALTER COLUMN session_id SET NOT NULL;

CREATE INDEX IF NOT EXISTS idx_auth_sessions_session_id
  ON auth_sessions(session_id);

CREATE INDEX IF NOT EXISTS idx_auth_sessions_account_last_seen
  ON auth_sessions(account_id, last_seen_at DESC);
