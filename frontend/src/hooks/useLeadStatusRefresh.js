import { useEffect } from 'react';

export const LEAD_STATUS_CHANGED = 'callified:lead-status-changed';

// Refresh only the changed row's status, preserving pagination and filters.
export default function useLeadStatusRefresh(setLeads, apiFetch, apiUrl, campaignId) {
  useEffect(() => {
    let active = true;
    const refresh = async event => {
      const id = event.detail?.leadId;
      if (!id) return;
      try {
        const query = campaignId ? `?campaign_id=${encodeURIComponent(campaignId)}` : '';
        const response = await apiFetch(`${apiUrl}/leads/${encodeURIComponent(id)}${query}`);
        if (!response.ok) return;
        const lead = await response.json();
        if (!active || !lead.status) return;
        setLeads(current => current.map(row => String(row.id) === String(id) ? { ...row, status: lead.status } : row));
      } catch { /* The regular lead refresh can retry after a network failure. */ }
    };
    window.addEventListener(LEAD_STATUS_CHANGED, refresh);
    return () => { active = false; window.removeEventListener(LEAD_STATUS_CHANGED, refresh); };
  }, [setLeads, apiFetch, apiUrl, campaignId]);
}
