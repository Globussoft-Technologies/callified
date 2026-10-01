package db

import (
	"fmt"
	"strings"
	"time"
)

type LeadCallStatusFilter struct {
	From         *time.Time
	To           *time.Time
	CampaignIDs  []int64
	AgentUserIDs []int64
	Dispositions []string
	Search       string
	LeadID       int64
	Page         int
	Limit        int
}

type LeadCallStatusRow struct {
	LeadID             int64   `json:"lead_id"`
	LeadName           string  `json:"lead_name"`
	Phone              string  `json:"phone"`
	CampaignID         int64   `json:"campaign_id"`
	CampaignName       string  `json:"campaign_name"`
	LastCallDate       string  `json:"last_call_date"`
	Disposition        string  `json:"disposition"`
	DispositionLabel   string  `json:"disposition_label"`
	DispositionSource  string  `json:"disposition_source"`
	AISummary          string  `json:"ai_summary"`
	AgentUserID        int64   `json:"agent_user_id"`
	AgentName          string  `json:"agent_name"`
	CallCount          int64   `json:"call_count"`
	TotalDurationS     float64 `json:"total_duration_s"`
	AverageDurationS   float64 `json:"average_duration_s"`
	LatestTranscriptID int64   `json:"latest_transcript_id"`
	LatestCallLogID    int64   `json:"latest_call_log_id"`
	TotalRows          int64   `json:"-"`
}

type LeadCallHistoryRow struct {
	DispositionID     int64   `json:"disposition_id"`
	LeadID            int64   `json:"lead_id"`
	CampaignID        int64   `json:"campaign_id"`
	CampaignName      string  `json:"campaign_name"`
	CallDate          string  `json:"call_date"`
	Disposition       string  `json:"disposition"`
	DispositionLabel  string  `json:"disposition_label"`
	DispositionSource string  `json:"disposition_source"`
	Summary           string  `json:"summary"`
	Sentiment         string  `json:"sentiment"`
	Confidence        float64 `json:"confidence"`
	Objections        string  `json:"objections"`
	NextAction        string  `json:"next_action"`
	AgentUserID       int64   `json:"agent_user_id"`
	AgentName         string  `json:"agent_name"`
	DurationS         float64 `json:"duration_s"`
	TranscriptID      int64   `json:"transcript_id"`
	CallLogID         int64   `json:"call_log_id"`
	CallSid           string  `json:"call_sid"`
	RecordingURL      string  `json:"recording_url"`
}

type ReportFilterOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

const reportEffectiveDisposition = `COALESCE(NULLIF(cd.final_disposition_code,''), cd.ai_disposition_code, 'callback')`
const reportCallDate = `COALESCE(ct.created_at, clog.created_at, cd.created_at)`

func addInt64Filter(where *strings.Builder, args *[]any, column string, values []int64) {
	if len(values) == 0 {
		return
	}
	where.WriteString(` AND ` + column + ` IN (` + strings.TrimSuffix(strings.Repeat("?,", len(values)), ",") + `)`)
	for _, value := range values {
		*args = append(*args, value)
	}
}

func addStringFilter(where *strings.Builder, args *[]any, column string, values []string) {
	if len(values) == 0 {
		return
	}
	where.WriteString(` AND ` + column + ` IN (` + strings.TrimSuffix(strings.Repeat("?,", len(values)), ",") + `)`)
	for _, value := range values {
		*args = append(*args, value)
	}
}

func leadCallStatusWhere(orgID int64, filter LeadCallStatusFilter, includeDisposition bool) (string, []any) {
	var where strings.Builder
	where.WriteString(` WHERE cd.org_id=? AND cd.lead_id IS NOT NULL`)
	args := []any{orgID}
	if filter.LeadID > 0 {
		where.WriteString(` AND cd.lead_id=?`)
		args = append(args, filter.LeadID)
	}
	if filter.From != nil {
		where.WriteString(` AND ` + reportCallDate + `>=?`)
		args = append(args, *filter.From)
	}
	if filter.To != nil {
		where.WriteString(` AND ` + reportCallDate + `<?`)
		args = append(args, *filter.To)
	}
	addInt64Filter(&where, &args, "cd.campaign_id", filter.CampaignIDs)
	addInt64Filter(&where, &args, "cd.agent_user_id", filter.AgentUserIDs)
	if includeDisposition {
		addStringFilter(&where, &args, reportEffectiveDisposition, filter.Dispositions)
	}
	if search := strings.TrimSpace(filter.Search); search != "" {
		like := "%" + search + "%"
		where.WriteString(` AND (l.first_name LIKE ? OR l.last_name LIKE ? OR l.phone LIKE ? OR l.company LIKE ?)`)
		args = append(args, like, like, like, like)
	}
	return where.String(), args
}

func (d *DB) GetLeadCallStatusReport(orgID int64, filter LeadCallStatusFilter) ([]LeadCallStatusRow, int64, error) {
	if filter.Page < 1 {
		filter.Page = 1
	}
	if filter.Limit < 1 {
		filter.Limit = 50
	}
	if filter.Limit > 100000 {
		filter.Limit = 100000
	}
	where, args := leadCallStatusWhere(orgID, filter, false)
	var latestWhere strings.Builder
	latestWhere.WriteString(` WHERE fc.row_num=1`)
	addStringFilter(&latestWhere, &args, "fc.disposition", filter.Dispositions)
	query := `WITH filtered_calls AS (
		SELECT cd.id, cd.lead_id, COALESCE(cd.campaign_id,0) campaign_id,
			COALESCE(c.name,'Not available') campaign_name,
			` + reportCallDate + ` call_date,
			` + reportEffectiveDisposition + ` disposition,
			COALESCE(NULLIF(cdo.label,''), ` + reportEffectiveDisposition + `) disposition_label,
			CASE WHEN COALESCE(cd.final_disposition_code,'')<>'' THEN 'human' ELSE 'ai' END disposition_source,
			COALESCE(NULLIF(cd.edited_summary,''), cd.ai_summary, '') summary,
			COALESCE(cd.agent_user_id,0) agent_user_id,
			COALESCE(NULLIF(u.full_name,''), u.email, 'Not available') agent_name,
			COALESCE(ct.call_duration_s,0) duration_s,
			COALESCE(cd.transcript_id,0) transcript_id, COALESCE(cd.call_log_id,0) call_log_id,
			ROW_NUMBER() OVER (PARTITION BY cd.lead_id ORDER BY ` + reportCallDate + ` DESC, cd.id DESC) row_num,
			COUNT(*) OVER (PARTITION BY cd.lead_id) call_count,
			SUM(COALESCE(ct.call_duration_s,0)) OVER (PARTITION BY cd.lead_id) total_duration
		FROM call_dispositions cd
		JOIN leads l ON l.id=cd.lead_id AND l.org_id=cd.org_id
		LEFT JOIN campaigns c ON c.id=cd.campaign_id AND c.org_id=cd.org_id
		LEFT JOIN call_logs clog ON clog.id=cd.call_log_id
		LEFT JOIN call_transcripts ct ON ct.id=cd.transcript_id
		LEFT JOIN users u ON u.id=cd.agent_user_id AND u.org_id=cd.org_id
		LEFT JOIN campaign_disposition_options cdo ON cdo.org_id=cd.org_id AND cdo.campaign_id=cd.campaign_id AND cdo.code=` + reportEffectiveDisposition + where + `
	), latest AS (
		SELECT fc.*, CONCAT_WS(' ', NULLIF(l.first_name,''), NULLIF(l.last_name,'')) lead_name,
			COALESCE(l.phone,'') phone
		FROM filtered_calls fc JOIN leads l ON l.id=fc.lead_id` + latestWhere.String() + `
	)
	SELECT lead_id, COALESCE(NULLIF(lead_name,''),'Unnamed lead'), phone, campaign_id, campaign_name,
		DATE_FORMAT(call_date,'%Y-%m-%d %H:%i:%s'), disposition, disposition_label, disposition_source,
		summary, agent_user_id, agent_name, call_count, total_duration,
		CASE WHEN call_count>0 THEN total_duration/call_count ELSE 0 END,
		transcript_id, call_log_id, COUNT(*) OVER()
	FROM latest ORDER BY call_date DESC, lead_id DESC LIMIT ? OFFSET ?`
	args = append(args, filter.Limit, (filter.Page-1)*filter.Limit)
	rows, err := d.pool.Query(query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("lead call status report: %w", err)
	}
	defer rows.Close()
	items := make([]LeadCallStatusRow, 0)
	var total int64
	for rows.Next() {
		var item LeadCallStatusRow
		if err := rows.Scan(&item.LeadID, &item.LeadName, &item.Phone, &item.CampaignID, &item.CampaignName,
			&item.LastCallDate, &item.Disposition, &item.DispositionLabel, &item.DispositionSource,
			&item.AISummary, &item.AgentUserID, &item.AgentName, &item.CallCount, &item.TotalDurationS,
			&item.AverageDurationS, &item.LatestTranscriptID, &item.LatestCallLogID, &item.TotalRows); err != nil {
			return nil, 0, err
		}
		total = item.TotalRows
		items = append(items, item)
	}
	return items, total, rows.Err()
}

func (d *DB) GetLeadCallStatusHistory(orgID int64, filter LeadCallStatusFilter) ([]LeadCallHistoryRow, error) {
	where, args := leadCallStatusWhere(orgID, filter, true)
	query := `SELECT cd.id, cd.lead_id, COALESCE(cd.campaign_id,0), COALESCE(c.name,'Not available'),
		DATE_FORMAT(` + reportCallDate + `,'%Y-%m-%d %H:%i:%s'), ` + reportEffectiveDisposition + `,
		COALESCE(NULLIF(cdo.label,''), ` + reportEffectiveDisposition + `),
		CASE WHEN COALESCE(cd.final_disposition_code,'')<>'' THEN 'human' ELSE 'ai' END,
		COALESCE(NULLIF(cd.edited_summary,''),cd.ai_summary,''), COALESCE(cd.ai_sentiment,'neutral'),
		COALESCE(cd.ai_confidence,0), COALESCE(cd.ai_objections,''), COALESCE(cd.ai_next_action,''),
		COALESCE(cd.agent_user_id,0), COALESCE(NULLIF(u.full_name,''),u.email,'Not available'),
		COALESCE(ct.call_duration_s,0), COALESCE(cd.transcript_id,0), COALESCE(cd.call_log_id,0),
		COALESCE(cd.call_sid,''), COALESCE(NULLIF(ct.recording_url,''),clog.recording_url,'')
	FROM call_dispositions cd
	JOIN leads l ON l.id=cd.lead_id AND l.org_id=cd.org_id
	LEFT JOIN campaigns c ON c.id=cd.campaign_id AND c.org_id=cd.org_id
	LEFT JOIN call_logs clog ON clog.id=cd.call_log_id
	LEFT JOIN call_transcripts ct ON ct.id=cd.transcript_id
	LEFT JOIN users u ON u.id=cd.agent_user_id AND u.org_id=cd.org_id
	LEFT JOIN campaign_disposition_options cdo ON cdo.org_id=cd.org_id AND cdo.campaign_id=cd.campaign_id AND cdo.code=` + reportEffectiveDisposition + where + `
	ORDER BY ` + reportCallDate + ` DESC, cd.id DESC LIMIT 500`
	rows, err := d.pool.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("lead call status history: %w", err)
	}
	defer rows.Close()
	items := make([]LeadCallHistoryRow, 0)
	for rows.Next() {
		var item LeadCallHistoryRow
		if err := rows.Scan(&item.DispositionID, &item.LeadID, &item.CampaignID, &item.CampaignName,
			&item.CallDate, &item.Disposition, &item.DispositionLabel, &item.DispositionSource,
			&item.Summary, &item.Sentiment, &item.Confidence, &item.Objections, &item.NextAction,
			&item.AgentUserID, &item.AgentName, &item.DurationS, &item.TranscriptID, &item.CallLogID,
			&item.CallSid, &item.RecordingURL); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (d *DB) GetLeadCallStatusDispositionOptions(orgID int64) ([]ReportFilterOption, error) {
	rows, err := d.pool.Query(`SELECT code, MAX(label) FROM (
		SELECT code, label FROM campaign_disposition_options WHERE org_id=? AND is_active=1
		UNION ALL
		SELECT COALESCE(NULLIF(final_disposition_code,''),ai_disposition_code),
			REPLACE(COALESCE(NULLIF(final_disposition_code,''),ai_disposition_code),'_',' ')
		FROM call_dispositions WHERE org_id=?
	) options WHERE code IS NOT NULL AND code<>'' GROUP BY code ORDER BY MAX(label)`, orgID, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var options []ReportFilterOption
	for rows.Next() {
		var option ReportFilterOption
		if err := rows.Scan(&option.Value, &option.Label); err != nil {
			return nil, err
		}
		options = append(options, option)
	}
	return options, rows.Err()
}
