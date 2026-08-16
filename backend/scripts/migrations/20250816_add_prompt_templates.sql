-- Phase 4: Prompt template registry + opening script FKs.
-- Run idempotently; duplicate column errors are expected to be ignored by callers.

CREATE TABLE IF NOT EXISTS prompt_templates (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    org_id BIGINT DEFAULT NULL,
    name VARCHAR(255) NOT NULL,
    language VARCHAR(10) DEFAULT 'en',
    template_type VARCHAR(50) DEFAULT 'voice',
    script_body LONGTEXT NOT NULL,
    version INT NOT NULL DEFAULT 1,
    is_active TINYINT(1) NOT NULL DEFAULT 1,
    created_at TIMESTAMP NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    INDEX idx_org_lang (org_id, language),
    INDEX idx_name (name),
    INDEX idx_active (is_active),
    FOREIGN KEY (org_id) REFERENCES organizations(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

ALTER TABLE products ADD COLUMN opening_script_id BIGINT DEFAULT NULL;
ALTER TABLE products ADD CONSTRAINT fk_products_opening_script
    FOREIGN KEY (opening_script_id) REFERENCES prompt_templates(id) ON DELETE SET NULL;

ALTER TABLE campaigns ADD COLUMN opening_script_id BIGINT DEFAULT NULL;
ALTER TABLE campaigns ADD CONSTRAINT fk_campaigns_opening_script
    FOREIGN KEY (opening_script_id) REFERENCES prompt_templates(id) ON DELETE SET NULL;
