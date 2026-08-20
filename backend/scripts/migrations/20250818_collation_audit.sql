-- Migration: standardise collations for exact-match identifier columns.
-- Run during a low-traffic window; rebuilding indexes on large tables may briefly lock.
--
-- Background
--   * Legacy Python tables used the server default (often utf8mb4_0900_ai_ci or
--     utf8mb4_general_ci). New Go tables used utf8mb4_0900_ai_ci explicitly.
--   * MySQL 8 extracts JSON strings as utf8mb4_0900_ai_ci, so joins against legacy
--     columns required an explicit COLLATE override in queries.
--   * Phone numbers and Twilio/Exotel call_sids need byte-exact matching.
--     Case-insensitive collations can mask subtle mismatches and waste index lookups.
--
-- This migration:
--   1. Switches Go-managed tables to utf8mb4_0900_ai_ci (MySQL 8 default) so JSON
--      extractions and table columns share a collation.
--   2. Re-creates exact-match identifier columns as utf8mb4_bin.
--   3. Converts the legacy leads/call_transcripts tables and fixes their key columns.
--   4. Rebuilds affected indexes.

-- ============================================================
-- 1. Helper procedures
-- ============================================================
DELIMITER $$

CREATE PROCEDURE IF NOT EXISTS convert_table_collation(
    IN p_table VARCHAR(128)
)
BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.tables
        WHERE table_schema = DATABASE()
          AND table_name = p_table
    ) THEN
        SET @sql = CONCAT('ALTER TABLE ', p_table,
                          ' CONVERT TO CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci');
        PREPARE stmt FROM @sql;
        EXECUTE stmt;
        DEALLOCATE PREPARE stmt;
    END IF;
END$$

CREATE PROCEDURE IF NOT EXISTS pin_column_collation(
    IN p_table VARCHAR(128),
    IN p_column VARCHAR(128),
    IN p_definition VARCHAR(512)
)
BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_schema = DATABASE()
          AND table_name = p_table
          AND column_name = p_column
    ) THEN
        SET @sql = CONCAT('ALTER TABLE ', p_table, ' MODIFY COLUMN ', p_column, ' ', p_definition);
        PREPARE stmt FROM @sql;
        EXECUTE stmt;
        DEALLOCATE PREPARE stmt;
    END IF;
END$$

CREATE PROCEDURE IF NOT EXISTS drop_index_if_exists(
    IN p_table VARCHAR(128),
    IN p_index VARCHAR(128)
)
BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.statistics
        WHERE table_schema = DATABASE()
          AND table_name = p_table
          AND index_name = p_index
    ) THEN
        SET @sql = CONCAT('ALTER TABLE ', p_table, ' DROP INDEX ', p_index);
        PREPARE stmt FROM @sql;
        EXECUTE stmt;
        DEALLOCATE PREPARE stmt;
    END IF;
END$$

DELIMITER ;

-- ============================================================
-- 2. Standardise table default collations to utf8mb4_0900_ai_ci
-- ============================================================
CALL convert_table_collation('call_logs');
CALL convert_table_collation('call_transcripts');
CALL convert_table_collation('leads');
CALL convert_table_collation('users');
CALL convert_table_collation('executives');
CALL convert_table_collation('campaigns');
CALL convert_table_collation('organizations');
CALL convert_table_collation('products');
CALL convert_table_collation('scheduled_calls');
CALL convert_table_collation('agent_activities');
CALL convert_table_collation('org_exotel_accounts');
CALL convert_table_collation('user_allowed_exotel_accounts');
CALL convert_table_collation('wa_channel_configs');
CALL convert_table_collation('whatsapp_conversations');
CALL convert_table_collation('whatsapp_messages');
CALL convert_table_collation('campaign_user_assignments');
CALL convert_table_collation('notifications');
CALL convert_table_collation('user_permissions');
CALL convert_table_collation('api_keys');
CALL convert_table_collation('prompt_templates');
CALL convert_table_collation('recording_dlq');
CALL convert_table_collation('webhook_dlq');

-- ============================================================
-- 3. Pin exact-match identifier columns to utf8mb4_bin
-- ============================================================

-- Phone numbers and call SIDs compare byte-for-byte.
CALL pin_column_collation('leads', 'phone', "VARCHAR(50) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL DEFAULT ''");
CALL pin_column_collation('call_logs', 'call_sid', "VARCHAR(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin DEFAULT NULL");
CALL pin_column_collation('call_logs', 'phone', "VARCHAR(50) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin DEFAULT NULL");
CALL pin_column_collation('call_transcripts', 'call_sid', "VARCHAR(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin DEFAULT NULL");
CALL pin_column_collation('whatsapp_conversations', 'phone', "VARCHAR(50) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL");
CALL pin_column_collation('wa_channel_configs', 'phone_number', "VARCHAR(50) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL");
CALL pin_column_collation('executives', 'phone', "VARCHAR(50) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin DEFAULT ''");

-- Provider identifiers are also exact.
CALL pin_column_collation('org_exotel_accounts', 'caller_id', "VARCHAR(50) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL DEFAULT ''");
CALL pin_column_collation('org_exotel_accounts', 'account_sid', "VARCHAR(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL DEFAULT ''");

-- Email remains case-insensitive for UX, but ensure it is at least consistent.
CALL pin_column_collation('users', 'email', "VARCHAR(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci NOT NULL DEFAULT ''");
CALL pin_column_collation('executives', 'email', "VARCHAR(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci DEFAULT ''");
CALL pin_column_collation('api_keys', 'api_key_hash', "VARCHAR(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_bin NOT NULL");

-- ============================================================
-- 4. Rebuild affected indexes so they use the new column collations
-- ============================================================
CALL drop_index_if_exists('call_logs', 'idx_call_sid');
ALTER TABLE call_logs ADD INDEX idx_call_sid (call_sid);

CALL drop_index_if_exists('leads', 'idx_leads_phone');
ALTER TABLE leads ADD INDEX idx_leads_phone (phone);

CALL drop_index_if_exists('users', 'idx_users_email');
ALTER TABLE users ADD INDEX idx_users_email (email);

-- ============================================================
-- 5. Clean up helper procedures
-- ============================================================
DROP PROCEDURE IF EXISTS convert_table_collation;
DROP PROCEDURE IF EXISTS pin_column_collation;
DROP PROCEDURE IF EXISTS drop_index_if_exists;
