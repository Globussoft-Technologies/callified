ALTER TABLE pronunciation_guide
    ADD COLUMN org_id INT NOT NULL DEFAULT 0 AFTER id;

ALTER TABLE pronunciation_guide
    ADD COLUMN created_at TIMESTAMP NULL DEFAULT CURRENT_TIMESTAMP;

-- Drop any legacy UNIQUE(word) index before applying this migration. The Go
-- startup migration detects and removes it automatically when present.
ALTER TABLE pronunciation_guide
    ADD UNIQUE INDEX uq_pronunciation_org_word (org_id, word);
