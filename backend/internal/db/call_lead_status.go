package db

// CompleteCallingLead applies specific latest-call outcomes to active leads.
// Generic contact updates cannot replace an existing business outcome, and
// closed/converted leads are left intact.
func (d *DB) CompleteCallingLead(orgID, leadID int64, callSID, status string) error {
	if orgID <= 0 || leadID <= 0 || callSID == "" || status == "" {
		return nil
	}
	_, err := d.pool.Exec(`UPDATE leads SET status=?
 WHERE org_id=? AND id=? AND (LOWER(TRIM(COALESCE(status,''))) IN ('','new','calling','contacted','insufficient credits')
 OR LOWER(TRIM(status)) LIKE 'call failed%'
 OR (? <> 'Contacted' AND LOWER(TRIM(COALESCE(status,''))) NOT IN ('closed','converted')))
 AND (SELECT MAX(id) FROM call_logs WHERE org_id=? AND lead_id=?) =
 (SELECT MAX(id) FROM call_logs WHERE org_id=? AND lead_id=? AND call_sid=?)`,
		status, orgID, leadID, status, orgID, leadID, orgID, leadID, callSID)
	return err
}

// SyncCallDispositionLeadStatus also handles cached reviews and human overrides
// using the effective outcome, so regenerating AI cannot undo a reviewed result.
func (d *DB) SyncCallDispositionLeadStatus(value *CallDisposition) error {
	if value == nil {
		return nil
	}
	return d.CompleteCallingLead(value.OrgID, value.LeadID, value.CallSid, leadStatusForCallDisposition(value.EffectiveCode))
}

func leadStatusForCallDisposition(code string) string {
	switch code {
	case "interested":
		return "Interested"
	case "not_interested":
		return "Not Interested"
	case "callback":
		return "Follow-up"
	case "appointment_booked":
		return "Appointment Set"
	case "no_answer":
		return "" // Record the attempt without resetting the master lead status.
	case "wrong_number":
		return "Junk"
	case "dnc":
		return "Lost"
	default:
		return "Contacted"
	}
}
