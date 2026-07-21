-- Migration: RBAC + campaign-user assignments + notifications
-- Applied automatically on startup by backend/internal/db/db.go

-- 1. Extend users table for manager relationship and active state
ALTER TABLE users ADD COLUMN manager_id BIGINT DEFAULT NULL;
ALTER TABLE users ADD COLUMN is_active TINYINT(1) NOT NULL DEFAULT 1;
CREATE INDEX idx_manager_id ON users(manager_id);

-- 2. Link campaigns to dashboard users (Admin assignments).
-- campaign_id/user_id are INT to match existing campaigns/users tables.
CREATE TABLE IF NOT EXISTS campaign_user_assignments (
  campaign_id INT NOT NULL,
  user_id INT NOT NULL,
  assigned_by INT DEFAULT NULL,
  assigned_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (campaign_id, user_id),
  INDEX idx_user_id (user_id),
  INDEX idx_campaign_id (campaign_id),
  FOREIGN KEY (campaign_id) REFERENCES campaigns(id) ON DELETE CASCADE,
  FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- 3. In-app notifications (persistent, mark-as-read)
CREATE TABLE IF NOT EXISTS notifications (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,
  user_id INT NOT NULL,
  type VARCHAR(50) NOT NULL,
  title VARCHAR(255) NOT NULL,
  body TEXT,
  payload JSON,
  is_read TINYINT(1) NOT NULL DEFAULT 0,
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  INDEX idx_user_id_read (user_id, is_read),
  INDEX idx_created_at (created_at DESC)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- 4. Backfill existing non-admin users to Agent
UPDATE users SET role='Agent' WHERE role NOT IN ('Admin','SuperAdmin');
