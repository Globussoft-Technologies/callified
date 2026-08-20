-- Migration: dead-letter queue for recording uploads that exhaust retries.
CREATE TABLE IF NOT EXISTS recording_dlq (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    org_id BIGINT NOT NULL,
    lead_id BIGINT DEFAULT NULL,
    campaign_id BIGINT DEFAULT NULL,
    call_sid VARCHAR(255) DEFAULT NULL,
    stream_sid VARCHAR(255) DEFAULT NULL,
    object_key TEXT NOT NULL,
    provider VARCHAR(50) NOT NULL,
    attempts INT NOT NULL DEFAULT 0,
    last_error TEXT DEFAULT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    INDEX idx_recording_dlq_org (org_id),
    INDEX idx_recording_dlq_created (created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
