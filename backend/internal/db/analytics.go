package db

// ── Dashboard types ──────────────────────────────────────────────────────────

// FullDashboardStats is the complete analytics payload the frontend expects.
type FullDashboardStats struct {
	TotalCalls          int64              `json:"total_calls"`
	CallsToday          int64              `json:"calls_today"`
	CallsThisWeek       int64              `json:"calls_this_week"`
	PickupRate          float64            `json:"pickup_rate"`
	AppointmentRate     float64            `json:"appointment_rate"`
	AvgCallDurationSec  float64            `json:"avg_call_duration_sec"`
	DailyCalls          []DailyCallCount   `json:"daily_calls"`
	SentimentBreakdown  SentimentBreakdown `json:"sentiment_breakdown"`
	CampaignPerformance []CampaignPerf     `json:"campaign_performance"`
	TopFailureReasons   []FailureReason    `json:"top_failure_reasons"`
}

type DailyCallCount struct {
	Date  string `json:"date"`
	Count int64  `json:"count"`
}

type SentimentBreakdown struct {
	Positive int64 `json:"positive"`
	Neutral  int64 `json:"neutral"`
	Negative int64 `json:"negative"`
}

type CampaignPerf struct {
	CampaignID   int64   `json:"campaign_id"`
	Name         string  `json:"name"`
	Calls        int64   `json:"calls"`
	Appointments int64   `json:"appointments"`
	AvgScore     float64 `json:"avg_score"`
}

type FailureReason struct {
	Reason string `json:"reason"`
	Count  int64  `json:"count"`
}

// LanguagePerf holds enriched per-language call metrics.
type LanguagePerf struct {
	Language       string  `json:"language"`
	TotalCalls     int64   `json:"total_calls"`
	Appointments   int64   `json:"appointments"`
	ConversionRate float64 `json:"conversion_rate"`
	AvgScore       float64 `json:"avg_score"`
	AvgDuration    float64 `json:"avg_duration"`
}

// GetFullDashboardStats returns the complete analytics payload for an org.
func (d *DB) GetFullDashboardStats(orgID int64, execIDs []int64, applyExecFilter bool) (*FullDashboardStats, error) {
	s := &FullDashboardStats{
		DailyCalls:          []DailyCallCount{},
		CampaignPerformance: []CampaignPerf{},
		TopFailureReasons:   []FailureReason{},
	}
	filterClause, filterArgs := execFilterClause(execIDs, applyExecFilter)
	hasFilter := filterClause != ""

	// ── 1. Aggregate counts ───────────────────────────────────────────────────
	var totalCalls, callsToday, callsThisWeek, connected int64
	var avgDur float64
	aggQ := `SELECT
			COUNT(*),
			COALESCE(SUM(CASE WHEN DATE(ct.created_at)=CURDATE() THEN 1 ELSE 0 END),0),
			COALESCE(SUM(CASE WHEN ct.created_at>=DATE_SUB(NOW(),INTERVAL 6 DAY) THEN 1 ELSE 0 END),0),
			COALESCE(SUM(CASE WHEN ct.status NOT IN ('failed','no-answer','busy','initiated') THEN 1 ELSE 0 END),0),
			COALESCE(AVG(NULLIF(ct.call_duration_s,0)),0)
		FROM call_transcripts ct
		LEFT JOIN leads l ON ct.lead_id=l.id
		WHERE ct.org_id=?`
	aggArgs := []any{orgID}
	if hasFilter {
		aggQ += ` AND ` + filterClause
		aggArgs = append(aggArgs, filterArgs...)
	}
	err := d.pool.QueryRow(aggQ, aggArgs...).
		Scan(&totalCalls, &callsToday, &callsThisWeek, &connected, &avgDur)
	if err != nil {
		return s, err
	}

	// Appointments: distinct transcripts that have at least one review with
	// appointment_booked=1.
	apptQ := `SELECT COALESCE(COUNT(DISTINCT ct.id),0)
		FROM call_transcripts ct
		JOIN call_reviews cr ON cr.transcript_id=ct.id
		LEFT JOIN leads l ON ct.lead_id=l.id
		WHERE ct.org_id=? AND cr.appointment_booked=1`
	apptArgs := []any{orgID}
	if hasFilter {
		apptQ += ` AND ` + filterClause
		apptArgs = append(apptArgs, filterArgs...)
	}
	var appointments int64
	if err := d.pool.QueryRow(apptQ, apptArgs...).Scan(&appointments); err != nil {
		appointments = 0
	}
	s.TotalCalls = totalCalls
	s.CallsToday = callsToday
	s.CallsThisWeek = callsThisWeek
	s.AvgCallDurationSec = avgDur
	if totalCalls > 0 {
		s.PickupRate = float64(connected) / float64(totalCalls)
		s.AppointmentRate = float64(appointments) / float64(totalCalls)
	}

	// ── 2. Daily calls (last 7 days) ──────────────────────────────────────────
	dailyQ := `WITH RECURSIVE days AS (
			SELECT DATE_SUB(CURDATE(), INTERVAL 6 DAY) AS d
			UNION ALL
			SELECT DATE_ADD(d, INTERVAL 1 DAY) FROM days WHERE d < CURDATE()
		)
		SELECT DATE_FORMAT(days.d, '%Y-%m-%d') AS day,
		       COALESCE(COUNT(ct.id), 0) AS cnt
		FROM days
		LEFT JOIN call_transcripts ct
		  ON DATE(ct.created_at) = days.d AND ct.org_id = ?`
	dailyArgs := []any{orgID}
	if hasFilter {
		dailyQ += ` LEFT JOIN leads l ON l.id = ct.lead_id AND ` + filterClause
		dailyArgs = append(dailyArgs, filterArgs...)
	}
	dailyQ += ` GROUP BY days.d ORDER BY days.d ASC`
	rows, err := d.pool.Query(dailyQ, dailyArgs...)
	if err != nil {
		return s, err
	}
	defer rows.Close()
	for rows.Next() {
		var dc DailyCallCount
		if err := rows.Scan(&dc.Date, &dc.Count); err != nil {
			return s, err
		}
		s.DailyCalls = append(s.DailyCalls, dc)
	}
	if err := rows.Err(); err != nil {
		return s, err
	}

	// ── 3. Sentiment breakdown (from call_reviews) ────────────────────────────
	sentQ := `SELECT
			COALESCE(SUM(CASE WHEN cr.sentiment='positive' THEN 1 ELSE 0 END),0),
			COALESCE(SUM(CASE WHEN cr.sentiment='neutral' THEN 1 ELSE 0 END),0),
			COALESCE(SUM(CASE WHEN cr.sentiment='negative' THEN 1 ELSE 0 END),0)
		FROM call_reviews cr
		JOIN call_transcripts ct ON cr.transcript_id=ct.id
		LEFT JOIN leads l ON ct.lead_id=l.id
		WHERE ct.org_id=?`
	sentArgs := []any{orgID}
	if hasFilter {
		sentQ += ` AND ` + filterClause
		sentArgs = append(sentArgs, filterArgs...)
	}
	err = d.pool.QueryRow(sentQ, sentArgs...).
		Scan(&s.SentimentBreakdown.Positive, &s.SentimentBreakdown.Neutral, &s.SentimentBreakdown.Negative)
	if err != nil {
		s.SentimentBreakdown = SentimentBreakdown{}
	}

	// ── 4. Campaign performance ───────────────────────────────────────────────
	cpQ := `SELECT ct.campaign_id, c.name,
			COUNT(DISTINCT ct.id) AS calls,
			COUNT(DISTINCT CASE WHEN cr.appointment_booked=1 THEN ct.id END) AS appts,
			COALESCE(AVG(cr.quality_score),0) AS avg_score
		FROM call_transcripts ct
		JOIN campaigns c ON ct.campaign_id=c.id
		LEFT JOIN call_reviews cr ON cr.transcript_id=ct.id
		LEFT JOIN leads l ON ct.lead_id=l.id
		WHERE ct.org_id=? AND ct.campaign_id IS NOT NULL`
	cpArgs := []any{orgID}
	if hasFilter {
		cpQ += ` AND ` + filterClause
		cpArgs = append(cpArgs, filterArgs...)
	}
	cpQ += ` GROUP BY ct.campaign_id, c.name ORDER BY calls DESC LIMIT 20`
	cpRows, err := d.pool.Query(cpQ, cpArgs...)
	if err == nil {
		defer cpRows.Close()
		for cpRows.Next() {
			var cp CampaignPerf
			if err := cpRows.Scan(&cp.CampaignID, &cp.Name, &cp.Calls, &cp.Appointments, &cp.AvgScore); err == nil {
				s.CampaignPerformance = append(s.CampaignPerformance, cp)
			}
		}
	}

	// ── 5. Top failure reasons ────────────────────────────────────────────────
	frQ := `SELECT cr.failure_reason, COUNT(*) AS cnt
		FROM call_reviews cr
		JOIN call_transcripts ct ON cr.transcript_id=ct.id
		LEFT JOIN leads l ON ct.lead_id=l.id
		WHERE ct.org_id=? AND cr.failure_reason IS NOT NULL AND cr.failure_reason<>''`
	frArgs := []any{orgID}
	if hasFilter {
		frQ += ` AND ` + filterClause
		frArgs = append(frArgs, filterArgs...)
	}
	frQ += ` GROUP BY cr.failure_reason ORDER BY cnt DESC LIMIT 10`
	frRows, err := d.pool.Query(frQ, frArgs...)
	if err == nil {
		defer frRows.Close()
		for frRows.Next() {
			var fr FailureReason
			if err := frRows.Scan(&fr.Reason, &fr.Count); err == nil {
				s.TopFailureReasons = append(s.TopFailureReasons, fr)
			}
		}
	}

	return s, nil
}

// GetLanguagePerformance returns per-language call metrics for an org.
// COUNT(DISTINCT ct.id) is required because the LEFT JOIN to call_reviews
// produces one row per review — without DISTINCT, total_calls would be the
// sum of reviews instead of transcripts (issue #45).
//
// Older transcripts pre-date the tts_language column being written on each
// call; for those, fall back to the parent campaign's tts_language so the
// breakdown isn't dominated by an "unknown" bucket of historical data. Only
// transcripts whose campaign also has no language remain "unknown".
func (d *DB) GetLanguagePerformance(orgID int64, execIDs []int64, applyExecFilter bool) ([]LanguagePerf, error) {
	filterClause, filterArgs := execFilterClause(execIDs, applyExecFilter)
	hasFilter := filterClause != ""
	q := `SELECT
			COALESCE(NULLIF(ct.tts_language,''), NULLIF(c.tts_language,''), 'unknown') AS language,
			COUNT(DISTINCT ct.id) AS total_calls,
			COUNT(DISTINCT CASE WHEN ct.appointment_booked=1 THEN ct.id END) AS appointments,
			COALESCE(AVG(NULLIF(ct.call_duration_s,0)),0) AS avg_duration,
			COALESCE(AVG(cr.quality_score),0) AS avg_score
		FROM call_transcripts ct
		LEFT JOIN campaigns c ON ct.campaign_id=c.id
		LEFT JOIN call_reviews cr ON cr.transcript_id=ct.id
		LEFT JOIN leads l ON ct.lead_id=l.id
		WHERE ct.org_id=?`
	args := []any{orgID}
	if hasFilter {
		q += ` AND ` + filterClause
		args = append(args, filterArgs...)
	}
	q += ` GROUP BY language ORDER BY total_calls DESC`
	rows, err := d.pool.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []LanguagePerf
	for rows.Next() {
		var lp LanguagePerf
		if err := rows.Scan(&lp.Language, &lp.TotalCalls, &lp.Appointments, &lp.AvgDuration, &lp.AvgScore); err != nil {
			return nil, err
		}
		if lp.TotalCalls > 0 {
			lp.ConversionRate = float64(lp.Appointments) / float64(lp.TotalCalls) * 100
		}
		list = append(list, lp)
	}
	return list, rows.Err()
}

// CampaignExportRow holds per-lead call data for CSV/report export.
type CampaignExportRow struct {
	LeadName        string  `json:"lead_name"`
	Phone           string  `json:"phone"`
	Status          string  `json:"status"`
	CallDuration    int     `json:"call_duration_s"`
	SentimentScore  float64 `json:"sentiment_score"`
	AppointmentDate string  `json:"appointment_date"`
	FollowUpNote    string  `json:"follow_up_note"`
	CalledAt        string  `json:"called_at"`
}

// GetCampaignAnalyticsForExport returns one row per call transcript in a campaign
// (or across all org campaigns when campaignID == 0).
func (d *DB) GetCampaignAnalyticsForExport(orgID, campaignID int64, execIDs []int64, applyExecFilter bool) ([]CampaignExportRow, error) {
	filterClause, filterArgs := execFilterClause(execIDs, applyExecFilter)
	hasFilter := filterClause != ""
	q := `
		SELECT
			CONCAT(l.first_name,' ',COALESCE(l.last_name,'')) AS lead_name,
			l.phone,
			COALESCE(l.status,'new'),
			COALESCE(ct.call_duration_s,0),
			COALESCE(ct.sentiment_score,0),
			COALESCE(ct.appointment_date,''),
			COALESCE(l.follow_up_note,''),
			DATE_FORMAT(ct.created_at,'%Y-%m-%d %H:%i:%s')
		FROM call_transcripts ct
		JOIN leads l ON ct.lead_id=l.id
		WHERE ct.org_id=?`
	args := []any{orgID}
	if campaignID > 0 {
		q += ` AND ct.campaign_id=?`
		args = append(args, campaignID)
	}
	if hasFilter {
		q += ` AND ` + filterClause
		args = append(args, filterArgs...)
	}
	q += ` ORDER BY ct.id DESC`
	rows, err := d.pool.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []CampaignExportRow
	for rows.Next() {
		var r CampaignExportRow
		if err := rows.Scan(&r.LeadName, &r.Phone, &r.Status,
			&r.CallDuration, &r.SentimentScore, &r.AppointmentDate,
			&r.FollowUpNote, &r.CalledAt); err != nil {
			return nil, err
		}
		list = append(list, r)
	}
	return list, rows.Err()
}

// ScoredLead is a lead with an AI quality score from call_reviews.
type ScoredLead struct {
	LeadID        int64   `json:"lead_id"`
	LeadName      string  `json:"lead_name"`
	Phone         string  `json:"phone"`
	Status        string  `json:"status"`
	QualityScore  float64 `json:"quality_score"`
	Sentiment     string  `json:"sentiment"`
	FailureReason string  `json:"failure_reason"`
	ReviewedAt    string  `json:"reviewed_at"`
}

// GetScoredLeads returns leads with AI quality scores for the given org/campaign.
// Pass campaignID=0 to get all scored leads for the org.
func (d *DB) GetScoredLeads(orgID, campaignID int64, execIDs []int64, applyExecFilter bool) ([]ScoredLead, error) {
	filterClause, filterArgs := execFilterClause(execIDs, applyExecFilter)
	hasFilter := filterClause != ""
	query := `
		SELECT l.id, CONCAT(l.first_name,' ',COALESCE(l.last_name,'')) AS name,
			l.phone, COALESCE(l.status,''),
			COALESCE(cr.quality_score,0), COALESCE(cr.sentiment,''),
			COALESCE(cr.failure_reason,''),
			DATE_FORMAT(cr.created_at,'%Y-%m-%d %H:%i:%s')
		FROM call_reviews cr
		JOIN call_transcripts ct ON cr.transcript_id=ct.id
		JOIN leads l ON ct.lead_id=l.id
		WHERE ct.org_id=?`
	args := []any{orgID}
	if campaignID > 0 {
		query += ` AND ct.campaign_id=?`
		args = append(args, campaignID)
	}
	if hasFilter {
		query += ` AND ` + filterClause
		args = append(args, filterArgs...)
	}
	query += ` ORDER BY cr.quality_score DESC, cr.id DESC LIMIT 500`

	rows, err := d.pool.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []ScoredLead
	for rows.Next() {
		var s ScoredLead
		if err := rows.Scan(&s.LeadID, &s.LeadName, &s.Phone, &s.Status,
			&s.QualityScore, &s.Sentiment, &s.FailureReason, &s.ReviewedAt); err != nil {
			return nil, err
		}
		list = append(list, s)
	}
	return list, rows.Err()
}
