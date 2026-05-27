ALTER TABLE admin_mail_config
  ADD COLUMN IF NOT EXISTS security jsonb NOT NULL DEFAULT '{
    "username": "admin",
    "passwordSet": false,
    "apiTokenSet": false
  }'::jsonb;
