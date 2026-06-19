-- Follow-up migration: additional tables/columns discovered when starting the Go backend.

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

DELIMITER ;

-- scheduled_calls.notes
CALL add_col_if_missing('scheduled_calls', 'notes', 'TEXT DEFAULT NULL');

-- sites: address, latitude, longitude, radius_m
CALL add_col_if_missing('sites', 'address', 'TEXT DEFAULT NULL');
CALL add_col_if_missing('sites', 'latitude', 'VARCHAR(50) DEFAULT NULL');
CALL add_col_if_missing('sites', 'longitude', 'VARCHAR(50) DEFAULT NULL');
CALL add_col_if_missing('sites', 'radius_m', 'INT DEFAULT 100');

-- Mirror legacy lat/lon into latitude/longitude for existing rows.
UPDATE sites SET latitude = CAST(lat AS CHAR), longitude = CAST(lon AS CHAR) WHERE latitude IS NULL;

-- punches: user_id, latitude, longitude, notes
CALL add_col_if_missing('punches', 'user_id', 'BIGINT DEFAULT NULL');
CALL add_col_if_missing('punches', 'latitude', 'DOUBLE DEFAULT NULL');
CALL add_col_if_missing('punches', 'longitude', 'DOUBLE DEFAULT NULL');
CALL add_col_if_missing('punches', 'notes', 'TEXT DEFAULT NULL');

-- demo_requests: name, company, message (legacy Python uses first_name/last_name/request_type)
CALL add_col_if_missing('demo_requests', 'name', 'VARCHAR(255) DEFAULT NULL');
CALL add_col_if_missing('demo_requests', 'company', 'VARCHAR(255) DEFAULT NULL');
CALL add_col_if_missing('demo_requests', 'message', 'TEXT DEFAULT NULL');

-- Seed name from legacy first_name/last_name where possible.
UPDATE demo_requests SET name = TRIM(CONCAT(COALESCE(first_name,''), ' ', COALESCE(last_name,'')))
  WHERE name IS NULL AND (first_name IS NOT NULL OR last_name IS NOT NULL);

-- team_invites table (missing on this production DB)
CREATE TABLE IF NOT EXISTS team_invites (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,
  token VARCHAR(255) NOT NULL,
  org_id BIGINT NOT NULL,
  email VARCHAR(255) NOT NULL,
  full_name VARCHAR(255) DEFAULT NULL,
  role VARCHAR(50) DEFAULT 'Member',
  invited_by_user_id BIGINT DEFAULT NULL,
  expires_at DATETIME NOT NULL,
  accepted_at DATETIME DEFAULT NULL,
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  UNIQUE KEY uk_team_invites_token (token),
  INDEX idx_team_invites_org_email (org_id, email),
  INDEX idx_team_invites_expires (expires_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
