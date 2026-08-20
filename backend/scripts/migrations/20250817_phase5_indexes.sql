-- Migration: add Phase 5 composite indexes for lead/campaign/agent filtering
-- and call-log analytics. These indexes support the campaign detail page,
-- agent-specific lead views, and report queries.

-- Speed up campaign detail page lead filtering by org + campaign + status.
CREATE INDEX IF NOT EXISTS idx_leads_org_campaign_status
    ON leads (org_id, campaign_id, status);

-- Speed up "my leads" / agent-specific lead listings.
CREATE INDEX IF NOT EXISTS idx_leads_org_executive_status
    ON leads (org_id, executive_id, status);

-- Speed up call-log analytics ordered by campaign and time.
CREATE INDEX IF NOT EXISTS idx_call_logs_campaign_created
    ON call_logs (campaign_id, created_at);

-- Speed up org-scoped user role lookups (RBAC, team management).
CREATE INDEX IF NOT EXISTS idx_users_org_role
    ON users (org_id, role);
