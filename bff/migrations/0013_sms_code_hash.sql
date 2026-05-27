ALTER TABLE sms_code_logs
  ADD COLUMN IF NOT EXISTS code_hash text NOT NULL DEFAULT '';

ALTER TABLE sms_code_logs
  ADD COLUMN IF NOT EXISTS code_masked text NOT NULL DEFAULT '';
