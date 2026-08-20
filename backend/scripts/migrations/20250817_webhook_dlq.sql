-- Migration: dead-letter queue for webhooks that exhaust retries.
CREATE TABLE IF NOT EXISTS webhook_dlq (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    webhook_id BIGINT NOT NULL,
    org_id BIGINT NOT NULL,
    event VARCHAR(100) NOT NULL,
    payload TEXT NOT NULL,
    last_status_code INT DEFAULT NULL,
    last_response TEXT DEFAULT NULL,
    attempts INT NOT NULL DEFAULT 0,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    INDEX idx_webhook_dlq_org (org_id),
    INDEX idx_webhook_dlq_webhook (webhook_id),
    INDEX idx_webhook_dlq_created (created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
