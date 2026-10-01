CREATE TABLE IF NOT EXISTS campaign_disposition_options (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    org_id BIGINT NOT NULL,
    campaign_id BIGINT NOT NULL,
    code VARCHAR(64) NOT NULL,
    label VARCHAR(100) NOT NULL,
    color VARCHAR(20) NOT NULL DEFAULT '#64748b',
    sort_order INT NOT NULL DEFAULT 0,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    UNIQUE KEY uq_campaign_disposition_code (campaign_id, code),
    KEY idx_campaign_disposition_org (org_id, campaign_id, is_active)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS call_dispositions (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    org_id BIGINT NOT NULL,
    campaign_id BIGINT DEFAULT NULL,
    lead_id BIGINT DEFAULT NULL,
    call_log_id BIGINT DEFAULT NULL,
    transcript_id BIGINT DEFAULT NULL,
    call_sid VARCHAR(255) DEFAULT NULL,
    agent_user_id BIGINT DEFAULT NULL,
    ai_disposition_code VARCHAR(64) NOT NULL DEFAULT 'callback',
    ai_summary TEXT,
    ai_sentiment VARCHAR(32) NOT NULL DEFAULT 'neutral',
    ai_confidence DECIMAL(5,4) NOT NULL DEFAULT 0,
    ai_objections TEXT,
    ai_next_action TEXT,
    final_disposition_code VARCHAR(64) DEFAULT NULL,
    edited_summary TEXT,
    override_reason TEXT,
    overridden_by BIGINT DEFAULT NULL,
    overridden_at TIMESTAMP NULL DEFAULT NULL,
    analyzed_at TIMESTAMP NULL DEFAULT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    UNIQUE KEY uq_call_disposition_log (call_log_id),
    UNIQUE KEY uq_call_disposition_transcript (transcript_id),
    KEY idx_call_disposition_lead (org_id, lead_id, created_at),
    KEY idx_call_disposition_campaign (org_id, campaign_id, created_at),
    KEY idx_call_disposition_sid (call_sid)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS call_disposition_events (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    disposition_id BIGINT NOT NULL,
    org_id BIGINT NOT NULL,
    user_id BIGINT NOT NULL,
    previous_code VARCHAR(64) DEFAULT NULL,
    new_code VARCHAR(64) NOT NULL,
    previous_summary TEXT,
    new_summary TEXT,
    reason TEXT,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    KEY idx_disposition_events_record (disposition_id, created_at),
    KEY idx_disposition_events_org (org_id, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

ALTER TABLE call_logs ADD COLUMN IF NOT EXISTS agent_user_id BIGINT DEFAULT NULL;
