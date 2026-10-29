package db

// Forwarded calls have a durable call log, but no AI transcript.
func (d *DB) GetRecentForwardedIncomingCalls(orgID int64, limit int) ([]InboundReceptionistCall, error) {
	rows, err := d.pool.Query(`SELECT cl.id,COALESCE(l.id,0),COALESCE(l.first_name,''),COALESCE(l.last_name,''),COALESCE(cl.phone,''),cl.status,DATE_FORMAT(cl.created_at,'%Y-%m-%d %H:%i:%s'),COALESCE(NULLIF(CONVERT(s.destination_name USING utf8mb4) COLLATE utf8mb4_unicode_ci,''),CONVERT(n.name USING utf8mb4) COLLATE utf8mb4_unicode_ci,''),s.phone
 FROM call_logs cl
 JOIN incoming_route_snapshots s ON s.org_id=cl.org_id AND BINARY s.call_sid=BINARY cl.call_sid AND s.mode='phone'
 LEFT JOIN org_forwarding_numbers n ON n.org_id=s.org_id AND BINARY n.phone=BINARY s.phone
 LEFT JOIN leads l ON l.id=(SELECT m.id FROM leads m WHERE m.org_id=cl.org_id AND BINARY REPLACE(REPLACE(REPLACE(m.phone,'+',''),' ',''),'-','') IN (BINARY REPLACE(cl.phone,'+',''),BINARY CONCAT('91',REPLACE(cl.phone,'+','')),BINARY IF(LEFT(REPLACE(cl.phone,'+',''),2)='91' AND LENGTH(REPLACE(cl.phone,'+',''))=12,SUBSTRING(REPLACE(cl.phone,'+',''),3),'')) ORDER BY m.id LIMIT 1)
 WHERE cl.org_id=? AND cl.provider='plivo-inbound' AND cl.status<>'in-progress'
 ORDER BY cl.created_at DESC,cl.id DESC LIMIT ?`, orgID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []InboundReceptionistCall{}
	for rows.Next() {
		c := InboundReceptionistCall{HandlingMode: "phone", Transcript: []byte("[]")}
		if err := rows.Scan(&c.CallLogID, &c.LeadID, &c.FirstName, &c.LastName, &c.Phone, &c.Status, &c.CreatedAt, &c.ForwardedName, &c.ForwardedPhone); err != nil {
			return nil, err
		}
		result = append(result, c)
	}
	return result, rows.Err()
}
