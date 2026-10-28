-- Smartflo authorization tokens exceed the legacy 200/255-character limits.
-- Widen storage without changing existing credential values or nullability.
ALTER TABLE org_exotel_accounts MODIFY COLUMN api_token TEXT NOT NULL;
ALTER TABLE campaigns MODIFY COLUMN exotel_api_token TEXT NULL;
