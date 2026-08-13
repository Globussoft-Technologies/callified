-- Add per-user ownership to org_exotel_accounts so Agents/TeamLeaders can use
-- their own provider credentials instead of the org/campaign default.
-- user_id = NULL  -> org-level account (existing behaviour).
-- user_id != NULL -> account owned by that user, scoped to the same org.

ALTER TABLE org_exotel_accounts
    ADD COLUMN IF NOT EXISTS user_id BIGINT DEFAULT NULL
        AFTER org_id,
    ADD INDEX IF NOT EXISTS idx_user_org (user_id, org_id),
    ADD CONSTRAINT IF NOT EXISTS fk_user_id
        FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE;
