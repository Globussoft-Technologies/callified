-- Migration: make the Python-created production schema compatible with the Go backend.
-- Run against app_callified_db before starting callified-go-audio on app.callified.ai.
-- This script is additive: it creates missing tables/columns and seeds default data.

DELIMITER $$

CREATE PROCEDURE IF NOT EXISTS add_col_if_missing(
    IN p_table VARCHAR(128),
    IN p_column VARCHAR(128),
    IN p_definition VARCHAR(512)
)
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_schema = DATABASE()
          AND table_name = p_table
          AND column_name = p_column
    ) THEN
        SET @sql = CONCAT('ALTER TABLE ', p_table, ' ADD COLUMN ', p_column, ' ', p_definition);
        PREPARE stmt FROM @sql;
        EXECUTE stmt;
        DEALLOCATE PREPARE stmt;
    END IF;
END$$

CREATE PROCEDURE IF NOT EXISTS add_index_if_missing(
    IN p_table VARCHAR(128),
    IN p_index VARCHAR(128),
    IN p_definition VARCHAR(512)
)
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.statistics
        WHERE table_schema = DATABASE()
          AND table_name = p_table
          AND index_name = p_index
    ) THEN
        SET @sql = CONCAT('ALTER TABLE ', p_table, ' ', p_definition);
        PREPARE stmt FROM @sql;
        EXECUTE stmt;
        DEALLOCATE PREPARE stmt;
    END IF;
END$$

CREATE PROCEDURE IF NOT EXISTS add_fk_if_missing(
    IN p_table VARCHAR(128),
    IN p_constraint VARCHAR(128),
    IN p_definition VARCHAR(512)
)
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.table_constraints
        WHERE table_schema = DATABASE()
          AND table_name = p_table
          AND constraint_name = p_constraint
    ) THEN
        SET @sql = CONCAT('ALTER TABLE ', p_table, ' ', p_definition);
        PREPARE stmt FROM @sql;
        EXECUTE stmt;
        DEALLOCATE PREPARE stmt;
    END IF;
END$$

DELIMITER ;

-- ============================================================
-- 1. Add columns required by Go to existing Python tables
-- ============================================================

CALL add_col_if_missing('leads', 'dial_attempts', 'INT DEFAULT 0');

CALL add_col_if_missing('call_transcripts', 'org_id', 'INT DEFAULT NULL');
CALL add_col_if_missing('call_transcripts', 'tts_language', 'VARCHAR(10) DEFAULT NULL');
CALL add_col_if_missing('call_transcripts', 'status', 'VARCHAR(50) DEFAULT NULL');
CALL add_col_if_missing('call_transcripts', 'appointment_booked', 'BOOLEAN DEFAULT FALSE');
CALL add_col_if_missing('call_transcripts', 'sentiment_score', 'FLOAT DEFAULT 0');
CALL add_col_if_missing('call_transcripts', 'appointment_date', 'VARCHAR(100) DEFAULT NULL');

-- Back-fill org_id on existing transcripts so billing usage is accurate.
UPDATE call_transcripts ct
  LEFT JOIN leads l ON ct.lead_id = l.id
  LEFT JOIN campaigns c ON ct.campaign_id = c.id
  SET ct.org_id = COALESCE(l.org_id, c.org_id)
  WHERE ct.org_id IS NULL;

CALL add_col_if_missing('call_reviews', 'org_id', 'INT DEFAULT NULL');
CALL add_col_if_missing('call_reviews', 'sentiment', 'VARCHAR(50) DEFAULT NULL');
CALL add_col_if_missing('call_reviews', 'summary', 'TEXT DEFAULT NULL');
CALL add_col_if_missing('call_reviews', 'insights', 'TEXT DEFAULT NULL');

-- Seed new columns from legacy Python columns where possible.
UPDATE call_reviews SET sentiment = customer_sentiment
  WHERE sentiment IS NULL AND customer_sentiment IS NOT NULL;

CALL add_col_if_missing('scheduled_calls', 'scheduled_at', 'DATETIME DEFAULT NULL');

UPDATE scheduled_calls SET scheduled_at = scheduled_time WHERE scheduled_at IS NULL;

CALL add_col_if_missing('call_retries', 'attempts', 'INT DEFAULT NULL');
CALL add_col_if_missing('call_retries', 'next_attempt_at', 'DATETIME DEFAULT NULL');

UPDATE call_retries SET attempts = attempt_number, next_attempt_at = retry_after
  WHERE attempts IS NULL;

CALL add_col_if_missing('webhooks', 'event', 'VARCHAR(100) DEFAULT NULL');
CALL add_col_if_missing('webhooks', 'secret_key', 'VARCHAR(255) DEFAULT NULL');

-- If Python stored a JSON array in `events`, copy the first element to the Go column.
UPDATE webhooks SET event = JSON_UNQUOTE(JSON_EXTRACT(events, '$[0]'))
  WHERE event IS NULL AND JSON_VALID(events) AND JSON_LENGTH(events) > 0;

CALL add_col_if_missing('webhook_logs', 'status_code', 'INT DEFAULT NULL');
CALL add_col_if_missing('webhook_logs', 'response', 'TEXT DEFAULT NULL');

UPDATE webhook_logs SET status_code = response_status, response = response_body
  WHERE status_code IS NULL;

-- ============================================================
-- 2. Create tables the Go backend needs but does not auto-create
-- ============================================================

CREATE TABLE IF NOT EXISTS call_logs (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,
  lead_id BIGINT DEFAULT NULL,
  campaign_id BIGINT DEFAULT NULL,
  org_id BIGINT NOT NULL,
  call_sid VARCHAR(255) DEFAULT NULL,
  provider VARCHAR(100) DEFAULT NULL,
  phone VARCHAR(50) DEFAULT NULL,
  status VARCHAR(50) DEFAULT NULL,
  recording_url TEXT DEFAULT NULL,
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  INDEX idx_call_sid (call_sid),
  INDEX idx_org_id (org_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS org_credits (
  org_id BIGINT PRIMARY KEY,
  balance_paise BIGINT NOT NULL DEFAULT 0,
  rate_per_min_paise INT DEFAULT 500,
  updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS credit_transactions (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,
  org_id BIGINT NOT NULL,
  delta_paise BIGINT NOT NULL,
  balance_after_paise BIGINT NOT NULL,
  type VARCHAR(50) DEFAULT NULL,
  reference VARCHAR(255) DEFAULT NULL,
  call_duration_s FLOAT DEFAULT NULL,
  rate_per_min_paise INT DEFAULT NULL,
  notes TEXT DEFAULT NULL,
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  INDEX idx_org_id (org_id),
  INDEX idx_reference (reference)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS wa_channel_configs (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,
  org_id BIGINT NOT NULL,
  provider VARCHAR(50) NOT NULL,
  phone_number VARCHAR(50) NOT NULL,
  api_key VARCHAR(512) DEFAULT NULL,
  app_id VARCHAR(512) DEFAULT NULL,
  webhook_url TEXT DEFAULT NULL,
  webhook_secret VARCHAR(255) DEFAULT NULL,
  credentials JSON DEFAULT NULL,
  default_product_id BIGINT DEFAULT NULL,
  is_active BOOLEAN DEFAULT TRUE,
  ai_enabled BOOLEAN DEFAULT TRUE,
  auto_reply BOOLEAN DEFAULT TRUE,
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  UNIQUE KEY uk_wa_channel_configs_org_provider_phone (org_id, provider, phone_number),
  INDEX idx_wa_channel_configs_org (org_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS whatsapp_conversations (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,
  org_id BIGINT NOT NULL,
  phone VARCHAR(50) NOT NULL,
  provider VARCHAR(50) DEFAULT NULL,
  last_message TEXT DEFAULT NULL,
  message_count INT DEFAULT 0,
  lead_id BIGINT DEFAULT NULL,
  is_muted BOOLEAN DEFAULT FALSE,
  is_archived BOOLEAN DEFAULT FALSE,
  ai_enabled BOOLEAN DEFAULT TRUE,
  updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  INDEX idx_whatsapp_conv_org_phone (org_id, phone),
  INDEX idx_whatsapp_conv_updated (updated_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS whatsapp_messages (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,
  conversation_id BIGINT NOT NULL,
  direction VARCHAR(20) DEFAULT 'inbound',
  message_text TEXT DEFAULT NULL,
  message_type VARCHAR(50) DEFAULT 'text',
  provider_msg_id VARCHAR(255) DEFAULT NULL,
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  UNIQUE KEY uk_whatsapp_msg_provider (provider_msg_id),
  INDEX idx_whatsapp_msg_conv (conversation_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS wa_blast_jobs (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,
  campaign_id BIGINT NOT NULL,
  org_id BIGINT NOT NULL,
  status VARCHAR(50) DEFAULT 'running',
  total INT DEFAULT 0,
  sent INT DEFAULT 0,
  failed INT DEFAULT 0,
  errors_json JSON DEFAULT NULL,
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  INDEX idx_wa_blast_campaign (campaign_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ============================================================
-- 3. Seed a default Exotel provider account per org and link voice campaigns
-- ============================================================

-- The Go backend resolves telephony credentials exclusively through
-- org_exotel_accounts linked by campaigns.exotel_account_id.
-- Create one default account per organization using the platform credentials.

CREATE TABLE IF NOT EXISTS org_exotel_accounts (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,
  org_id BIGINT NOT NULL,
  provider VARCHAR(50) DEFAULT 'exotel',
  name VARCHAR(255) NOT NULL,
  api_key VARCHAR(512) NOT NULL,
  api_token VARCHAR(512) NOT NULL,
  api_secret VARCHAR(512) DEFAULT '',
  account_sid VARCHAR(255) NOT NULL,
  caller_id VARCHAR(50) NOT NULL,
  app_id VARCHAR(255) DEFAULT '',
  app_type VARCHAR(20) DEFAULT 'exoml',
  created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
  INDEX idx_org_id (org_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

INSERT INTO org_exotel_accounts (org_id, provider, name, api_key, api_token, account_sid, caller_id, app_id, app_type)
SELECT o.id, 'exotel', 'Default Exotel', '4c5043337462f99685f986366569898a5cd2561727468003',
       '7b543b006abd55c89dcfecfd472915d8dc0147f2071b821c', 'globussoft3', '09513886363', '1210468', 'exoml'
FROM organizations o
LEFT JOIN org_exotel_accounts e ON e.org_id = o.id
WHERE e.id IS NULL;

-- Add the exotel_account_id column on campaigns if it does not exist yet.
CALL add_col_if_missing('campaigns', 'channel', "VARCHAR(20) NOT NULL DEFAULT 'voice'");
CALL add_col_if_missing('campaigns', 'exotel_account_id', 'BIGINT DEFAULT NULL');

-- Link existing voice campaigns that don't already have an account to the default account.
UPDATE campaigns c
JOIN org_exotel_accounts e ON e.org_id = c.org_id
SET c.exotel_account_id = e.id
WHERE (c.exotel_account_id IS NULL OR c.exotel_account_id = 0)
  AND (c.channel IS NULL OR c.channel = 'voice');

-- ============================================================
-- 4. Seed org credit rows so billing endpoints don't return empty state
-- ============================================================

INSERT INTO org_credits (org_id, balance_paise, rate_per_min_paise)
SELECT id, 0, 500 FROM organizations o
LEFT JOIN org_credits oc ON oc.org_id = o.id
WHERE oc.org_id IS NULL;
