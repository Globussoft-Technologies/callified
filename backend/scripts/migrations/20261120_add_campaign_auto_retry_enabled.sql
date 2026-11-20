-- Existing and new campaigns require explicit opt-in for automatic retries.
-- Startup also ensures this column for legacy databases.
ALTER TABLE campaigns ADD COLUMN auto_retry_enabled TINYINT(1) NOT NULL DEFAULT 0;
