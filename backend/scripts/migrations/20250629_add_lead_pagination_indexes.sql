-- Migration: add indexes to support server-side lead pagination
-- and replace correlated subqueries with JOIN aggregates.

-- Speed up global lead listings ordered by created_at
CREATE INDEX IF NOT EXISTS idx_leads_org_created ON leads (org_id, created_at DESC);

-- Speed up campaign-leads aggregates from call_transcripts
CREATE INDEX IF NOT EXISTS idx_ct_lead_campaign ON call_transcripts (lead_id, campaign_id);

-- Speed up pending scheduled call lookups per campaign lead
CREATE INDEX IF NOT EXISTS idx_sc_lead_campaign_status ON scheduled_calls (lead_id, campaign_id, status, scheduled_at);
