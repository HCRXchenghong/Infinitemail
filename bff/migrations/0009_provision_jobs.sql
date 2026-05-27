CREATE TABLE IF NOT EXISTS mailbox_provision_jobs (
  id text PRIMARY KEY,
  account_id text NOT NULL,
  email text NOT NULL,
  status text NOT NULL DEFAULT 'queued',
  attempts integer NOT NULL DEFAULT 0,
  last_error text NOT NULL DEFAULT '',
  next_run_at timestamptz,
  last_run_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  completed_at timestamptz
);

CREATE INDEX IF NOT EXISTS idx_mailbox_provision_jobs_status
  ON mailbox_provision_jobs(status);

CREATE UNIQUE INDEX IF NOT EXISTS idx_mailbox_provision_jobs_account_active
  ON mailbox_provision_jobs(account_id)
  WHERE status IN ('queued', 'running', 'blocked', 'failed');
