ALTER TABLE admin_mail_config
  ADD COLUMN IF NOT EXISTS ops jsonb NOT NULL DEFAULT '{
    "autoRunEnabled": false,
    "intervalMinutes": 5,
    "lastRunStatus": "idle",
    "lastRunMessage": "自动巡检未开启"
  }'::jsonb;
