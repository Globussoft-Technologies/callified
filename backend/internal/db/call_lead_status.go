package db

// CompleteCallingLead updates only the latest call's temporary Calling status.
// Agent-selected statuses and outcomes from newer calls take precedence.
func (d *DB) CompleteCallingLead(orgID, leadID int64, callSID, status string) error {
	if orgID <= 0 || leadID <= 0 || callSID == "" {
		return nil
	}
	_, err := d.pool.Exec(`UPDATE leads SET status=?
 WHERE org_id=? AND id=? AND LOWER(status)='calling'
 AND (SELECT MAX(id) FROM call_logs WHERE org_id=? AND lead_id=?) =
 (SELECT MAX(id) FROM call_logs WHERE org_id=? AND lead_id=? AND call_sid=?)`,
		status, orgID, leadID, orgID, leadID, orgID, leadID, callSID)
	return err
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
		return "New"
	case "wrong_number":
		return "Junk"
	case "dnc":
		return "Lost"
	default:
		return "Contacted"
	}
}
