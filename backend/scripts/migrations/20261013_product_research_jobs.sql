CREATE TABLE IF NOT EXISTS product_research_jobs (
 id VARCHAR(64) PRIMARY KEY, org_id BIGINT NOT NULL, product_id BIGINT NOT NULL,
 website_url TEXT NOT NULL, base_knowledge LONGTEXT NOT NULL,
 status VARCHAR(20) NOT NULL, extracted LONGTEXT NOT NULL, summary LONGTEXT NOT NULL,
 error_message TEXT NOT NULL, created_at TIMESTAMP(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
 updated_at TIMESTAMP(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
 INDEX idx_product_research (org_id, product_id, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
