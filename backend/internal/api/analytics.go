package api

import (
	"encoding/csv"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/globussoft/callified-backend/internal/db"
	"github.com/xuri/excelize/v2"
)

// GET /api/analytics/dashboard
// @Summary     Analytics dashboard
// @Description Returns full analytics including daily call counts, sentiment breakdown, campaign performance, and failure reasons. Requires Admin role.
// @Tags        analytics
// @Produce     json
// @Security    BearerAuth
// @Param       executive_ids  query  string  false  "Comma-separated executive IDs"
// @Success     200  {object}  db.FullDashboardStats
// @Failure     401  {object}  ErrorResponse
// @Failure     403  {object}  ErrorResponse
// @Failure     500  {object}  ErrorResponse
// @Router      /api/analytics/dashboard [get]
func (s *Server) analyticsDashboard(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, "reports.view") {
		return
	}
	ac := getAuth(r)
	execIDs, apply, err := s.resolveExecutiveIDs(r, ac)
	if err != nil {
		s.logger.Sugar().Errorw("analyticsDashboard", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	stats, err := s.db.GetFullDashboardStats(ac.OrgID, execIDs, apply)
	if err != nil {
		s.logger.Sugar().Errorw("analyticsDashboard", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

// GET /api/analytics/languages
// @Summary     Analytics: language performance
// @Description Returns call performance broken down by TTS language. Requires Admin role.
// @Tags        analytics
// @Produce     json
// @Security    BearerAuth
// @Param       executive_ids  query  string  false  "Comma-separated executive IDs"
// @Success     200  {array}   db.LanguagePerf
// @Failure     401  {object}  ErrorResponse
// @Failure     403  {object}  ErrorResponse
// @Failure     500  {object}  ErrorResponse
// @Router      /api/analytics/languages [get]
func (s *Server) analyticsLanguages(w http.ResponseWriter, r *http.Request) {
	ac := getAuth(r)
	execIDs, apply, err := s.resolveExecutiveIDs(r, ac)
	if err != nil {
		s.logger.Sugar().Errorw("analyticsLanguages", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	langs, err := s.db.GetLanguagePerformance(ac.OrgID, execIDs, apply)
	if err != nil {
		s.logger.Sugar().Errorw("analyticsLanguages", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, emptyJSON(langs))
}

// GET /api/analytics/export?campaign_id=N
// @Summary     Export analytics CSV
// @Description Exports campaign analytics as a downloadable CSV. Requires Admin role.
// @Tags        analytics
// @Produce     text/csv
// @Security    BearerAuth
// @Param       campaign_id    query  int64   false  "Campaign ID (0 = all campaigns)"
// @Param       executive_ids  query  string  false  "Comma-separated executive IDs"
// @Success     200  {file}    binary
// @Failure     401  {object}  ErrorResponse
// @Failure     403  {object}  ErrorResponse
// @Failure     500  {object}  ErrorResponse
// @Router      /api/analytics/export [get]
func (s *Server) analyticsExportCSV(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, "reports.download") {
		return
	}
	ac := getAuth(r)
	campaignIDStr := r.URL.Query().Get("campaign_id")
	campaignID, _ := strconv.ParseInt(campaignIDStr, 10, 64)

	if campaignID > 0 && !s.canViewCampaign(ac, campaignID) {
		writeError(w, http.StatusNotFound, "campaign not found")
		return
	}

	execIDs, apply, err := s.resolveExecutiveIDs(r, ac)
	if err != nil {
		s.logger.Sugar().Errorw("analyticsExportCSV", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	rows, err := s.db.GetCampaignAnalyticsForExport(ac.OrgID, campaignID, execIDs, apply)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=campaign_%d_export.csv", campaignID))

	cw := csv.NewWriter(w)
	_ = cw.Write([]string{"Lead Name", "Phone", "Status", "Call Duration (s)",
		"Sentiment Score", "Appointment Date", "Follow Up Note", "Called At"})
	for _, row := range rows {
		_ = cw.Write([]string{
			row.LeadName, row.Phone, row.Status,
			strconv.Itoa(row.CallDuration),
			fmt.Sprintf("%.2f", row.SentimentScore),
			row.AppointmentDate, row.FollowUpNote, row.CalledAt,
		})
	}
	cw.Flush()
}

// GET /api/analytics/report
// @Summary     Export analytics HTML report
// @Description Exports campaign analytics as an HTML report page. Requires Admin role.
// @Tags        analytics
// @Produce     text/html
// @Security    BearerAuth
// @Param       campaign_id    query  int64   false  "Campaign ID (0 = all campaigns)"
// @Param       executive_ids  query  string  false  "Comma-separated executive IDs"
// @Success     200  {string}  string  "HTML report"
// @Failure     401  {object}  ErrorResponse
// @Failure     403  {object}  ErrorResponse
// @Failure     500  {object}  ErrorResponse
// @Router      /api/analytics/report [get]
func (s *Server) analyticsExportReport(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, "reports.download") {
		return
	}
	ac := getAuth(r)
	campaignIDStr := r.URL.Query().Get("campaign_id")
	campaignID, _ := strconv.ParseInt(campaignIDStr, 10, 64)

	if campaignID > 0 && !s.canViewCampaign(ac, campaignID) {
		writeError(w, http.StatusNotFound, "campaign not found")
		return
	}

	execIDs, apply, err := s.resolveExecutiveIDs(r, ac)
	if err != nil {
		s.logger.Sugar().Errorw("analyticsExportReport", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	rows, err := s.db.GetCampaignAnalyticsForExport(ac.OrgID, campaignID, execIDs, apply)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, `<!DOCTYPE html><html><head><title>Campaign Report</title>
<style>body{font-family:sans-serif;margin:32px}table{border-collapse:collapse;width:100%%}
th,td{border:1px solid #ddd;padding:8px;font-size:13px}th{background:#f5f5f5}</style>
</head><body><h2>Campaign %d Report</h2>
<table><tr><th>Lead</th><th>Phone</th><th>Status</th><th>Duration (s)</th>
<th>Sentiment</th><th>Appointment</th><th>Called At</th></tr>`, campaignID)
	for _, row := range rows {
		fmt.Fprintf(w, `<tr><td>%s</td><td>%s</td><td>%s</td><td>%d</td>
<td>%.2f</td><td>%s</td><td>%s</td></tr>`,
			row.LeadName, row.Phone, row.Status, row.CallDuration,
			row.SentimentScore, row.AppointmentDate, row.CalledAt)
	}
	fmt.Fprint(w, "</table></body></html>")
}

// GET /api/analytics/scored-leads
// @Summary     List scored leads
// @Description Returns leads with AI-generated scores for a campaign. Requires Admin role.
// @Tags        analytics
// @Produce     json
// @Security    BearerAuth
// @Param       campaign_id    query  int64  false  "Campaign ID"
// @Param       executive_ids  query  string false  "Comma-separated executive IDs"
// @Success     200  {array}   object
// @Failure     401  {object}  ErrorResponse
// @Failure     403  {object}  ErrorResponse
// @Failure     500  {object}  ErrorResponse
// @Router      /api/analytics/scored-leads [get]
func (s *Server) scoredLeads(w http.ResponseWriter, r *http.Request) {
	ac := getAuth(r)
	campaignIDStr := r.URL.Query().Get("campaign_id")
	campaignID, _ := strconv.ParseInt(campaignIDStr, 10, 64)

	execIDs, apply, err := s.resolveExecutiveIDs(r, ac)
	if err != nil {
		s.logger.Sugar().Errorw("scoredLeads", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	leads, err := s.db.GetScoredLeads(ac.OrgID, campaignID, execIDs, apply)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, emptyJSON(leads))
}

// GET /api/analytics/agent-lead-summary
// @Summary     Agent lead summary
// @Description Returns assigned lead and call outcome summaries by agent/executive for the selected date range.
// @Tags        analytics
// @Produce     json
// @Security    BearerAuth
// @Param       from         query  string  false  "Start date (YYYY-MM-DD)"
// @Param       to           query  string  false  "End date (YYYY-MM-DD)"
// @Param       campaign_id  query  int64   false  "Campaign ID (0 = all)"
// @Param       user_id      query  int64   false  "Agent/user ID"
// @Success     200  {array} db.AgentLeadSummary
// @Failure     400  {object} ErrorResponse
// @Failure     401  {object} ErrorResponse
// @Failure     403  {object} ErrorResponse
// @Failure     500  {object} ErrorResponse
// @Router      /api/analytics/agent-lead-summary [get]
func (s *Server) agentLeadSummary(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, "reports.view") {
		return
	}
	ac := getAuth(r)
	fromStr := r.URL.Query().Get("from")
	toStr := r.URL.Query().Get("to")
	if fromStr == "" {
		fromStr = time.Now().UTC().Format("2006-01-02")
	}
	if toStr == "" {
		toStr = fromStr
	}
	from, err := time.Parse("2006-01-02", fromStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid from date")
		return
	}
	to, err := time.Parse("2006-01-02", toStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid to date")
		return
	}
	to = to.Add(24*time.Hour - time.Second)

	campaignID, _ := strconv.ParseInt(r.URL.Query().Get("campaign_id"), 10, 64)
	if campaignID > 0 && !s.canViewCampaign(ac, campaignID) {
		writeError(w, http.StatusNotFound, "campaign not found")
		return
	}

	userID, _ := strconv.ParseInt(r.URL.Query().Get("user_id"), 10, 64)
	if db.IsAgentLikeRole(ac.Role) {
		if userID != 0 && userID != ac.UserID {
			writeError(w, http.StatusForbidden, "forbidden")
			return
		}
		userID = ac.UserID
	}

	rows, err := s.db.GetAgentLeadSummary(ac.OrgID, from, to, campaignID, userID)
	if err != nil {
		s.logger.Sugar().Errorw("agentLeadSummary", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, emptyJSON(rows))
}

func formatSeconds(sec int64) string {
	if sec <= 0 {
		return "0s"
	}
	if sec < 60 {
		return fmt.Sprintf("%ds", sec)
	}
	m := sec / 60
	if m < 60 {
		return fmt.Sprintf("%dm", m)
	}
	h := m / 60
	return fmt.Sprintf("%dh %dm", h, m%60)
}

// GET /api/analytics/agent-report
// @Summary     Agent productivity Excel report
// @Description Returns an .xlsx workbook with multiple sheets: Summary, Call Activity, Efficiency, Outcomes, and Detail.
// @Tags        analytics
// @Produce     application/vnd.openxmlformats-officedocument.spreadsheetml.sheet
// @Security    BearerAuth
// @Param       from         query  string  false  "Start date (YYYY-MM-DD)"
// @Param       to           query  string  false  "End date (YYYY-MM-DD)"
// @Param       campaign_id  query  int64   false  "Campaign ID (0 = all)"
// @Param       user_id      query  int64   false  "Agent user ID (Agent role can only request self)"
// @Success     200  {file}  binary
// @Failure     400  {object}  ErrorResponse
// @Failure     401  {object}  ErrorResponse
// @Failure     500  {object}  ErrorResponse
// @Router      /api/analytics/agent-report [get]
func (s *Server) agentReportXLSX(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, "reports.download") {
		return
	}
	ac := getAuth(r)

	fromStr := r.URL.Query().Get("from")
	toStr := r.URL.Query().Get("to")
	if fromStr == "" {
		fromStr = time.Now().UTC().Format("2006-01-02")
	}
	if toStr == "" {
		toStr = time.Now().UTC().Format("2006-01-02")
	}
	from, err := time.Parse("2006-01-02", fromStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid from date")
		return
	}
	to, err := time.Parse("2006-01-02", toStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid to date")
		return
	}
	to = to.Add(24*time.Hour - time.Second)

	campaignIDStr := r.URL.Query().Get("campaign_id")
	campaignID, _ := strconv.ParseInt(campaignIDStr, 10, 64)

	userIDStr := r.URL.Query().Get("user_id")
	userID, _ := strconv.ParseInt(userIDStr, 10, 64)
	// Agents/Executives may only view their own report.
	if db.IsAgentLikeRole(ac.Role) {
		if userID != 0 && userID != ac.UserID {
			writeError(w, http.StatusForbidden, "forbidden")
			return
		}
		userID = ac.UserID
	}

	if campaignID > 0 && !s.canViewCampaign(ac, campaignID) {
		writeError(w, http.StatusNotFound, "campaign not found")
		return
	}

	summary, err := s.db.GetAgentActivitySummary(ac.OrgID, from, to, campaignID, userID)
	if err != nil {
		s.logger.Sugar().Errorw("agentReportXLSX", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	detail, err := s.db.GetAgentActivityDetail(ac.OrgID, from, to, campaignID, userID)
	if err != nil {
		s.logger.Sugar().Errorw("agentReportXLSX", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	f := excelize.NewFile()
	defer func() { _ = f.Close() }()

	// Helper to create a sheet and write rows.
	writeSheet := func(name string, headers []string, rows [][]any) {
		sheet, _ := f.NewSheet(name)
		for col, h := range headers {
			cell, _ := excelize.CoordinatesToCellName(col+1, 1)
			_ = f.SetCellValue(name, cell, h)
		}
		for rIdx, row := range rows {
			for cIdx, v := range row {
				cell, _ := excelize.CoordinatesToCellName(cIdx+1, rIdx+2)
				_ = f.SetCellValue(name, cell, v)
			}
		}
		f.SetActiveSheet(sheet)
	}

	// Helper to build an Excel range string.
	rangeRef := func(sheet, startCol, endCol string, startRow, endRow int) string {
		return fmt.Sprintf("'%s'!$%s$%d:$%s$%d", sheet, startCol, startRow, endCol, endRow)
	}

	// Summary sheet
	var summaryRows [][]any
	for _, a := range summary {
		summaryRows = append(summaryRows, []any{
			a.FullName, a.Email, a.Role, a.TotalCalls, a.Connected, a.Completed, a.Unanswered, a.Busy, a.Failed,
			formatSeconds(a.TotalTalkTime), formatSeconds(a.BreakTime), formatSeconds(a.IdleTime),
			a.StatusUpdates, a.NotesAdded, a.Appointments, a.Conversions,
		})
	}
	writeSheet("Summary", []string{
		"Agent Name", "Email", "Role", "Total Calls", "Connected", "Completed", "Unanswered", "Busy", "Failed",
		"Talk Time", "Break Time", "Idle Time", "Status Updates", "Notes Added", "Appointments", "Conversions",
	}, summaryRows)

	// Call Activity sheet
	var callRows [][]any
	for _, a := range summary {
		callRows = append(callRows, []any{
			a.FullName, a.TotalCalls, a.Connected, a.Completed, a.Unanswered, a.Busy, a.Failed,
		})
	}
	writeSheet("Call Activity", []string{
		"Agent Name", "Total Calls", "Connected", "Completed", "Unanswered", "Busy", "Failed",
	}, callRows)

	// Efficiency sheet (numeric helper columns for charting are hidden)
	var effRows [][]any
	workHours := 8.0
	for _, a := range summary {
		avgDur := 0.0
		if a.TotalCalls > 0 {
			avgDur = float64(a.TotalTalkTime) / float64(a.TotalCalls)
		}
		callsPerHour := 0.0
		if workHours > 0 {
			callsPerHour = float64(a.TotalCalls) / workHours
		}
		effRows = append(effRows, []any{
			a.FullName,
			formatSeconds(a.TotalTalkTime), a.TotalTalkTime,
			formatSeconds(int64(avgDur)), fmt.Sprintf("%.1f", callsPerHour),
			formatSeconds(a.IdleTime), a.IdleTime,
			a.StatusUpdates, a.NotesAdded,
		})
	}
	writeSheet("Efficiency", []string{
		"Agent Name", "Talk Time", "Talk Time (s)", "Avg Call Duration", "Calls / Hour", "Idle Time", "Idle Time (s)", "Status Updates", "Notes Added",
	}, effRows)
	_ = f.SetColWidth("Efficiency", "C", "C", 0)
	_ = f.SetColWidth("Efficiency", "G", "G", 0)

	// Outcomes sheet
	var outRows [][]any
	var totalAppointments, totalConversions, totalStatusUpdates, totalNotes int64
	for _, a := range summary {
		outRows = append(outRows, []any{
			a.FullName, a.Appointments, a.Conversions, a.StatusUpdates, a.NotesAdded,
		})
		totalAppointments += a.Appointments
		totalConversions += a.Conversions
		totalStatusUpdates += a.StatusUpdates
		totalNotes += a.NotesAdded
	}
	writeSheet("Outcomes", []string{
		"Agent Name", "Appointments", "Conversions", "Status Updates", "Notes Added",
	}, outRows)
	// Totals table for the doughnut chart.
	outcomeLabels := []string{"Appointments", "Conversions", "Status Updates", "Notes Added"}
	outcomeTotals := []int64{totalAppointments, totalConversions, totalStatusUpdates, totalNotes}
	for i, label := range outcomeLabels {
		_ = f.SetCellValue("Outcomes", fmt.Sprintf("G%d", i+2), label)
		_ = f.SetCellValue("Outcomes", fmt.Sprintf("H%d", i+2), outcomeTotals[i])
	}

	// Detail sheet
	var detailRows [][]any
	for _, d := range detail {
		detailRows = append(detailRows, []any{
			d.CreatedAt, d.UserID, d.ActivityType, d.CampaignID, d.LeadID, fmt.Sprintf("%v", d.Metadata),
		})
	}
	writeSheet("Detail", []string{
		"Timestamp", "User ID", "Activity Type", "Campaign ID", "Lead ID", "Metadata",
	}, detailRows)

	// ── Charts ─────────────────────────────────────────────────────────────────
	if len(summary) > 0 {
		lastRow := len(summary) + 1

		// Summary: clustered column of Total Calls, Status Updates, Notes Added.
		_ = f.AddChart("Summary", "R2", &excelize.Chart{
			Type: excelize.Col,
			Series: []excelize.ChartSeries{
				{Name: "Total Calls", Categories: rangeRef("Summary", "A", "A", 2, lastRow), Values: rangeRef("Summary", "D", "D", 2, lastRow)},
				{Name: "Status Updates", Categories: rangeRef("Summary", "A", "A", 2, lastRow), Values: rangeRef("Summary", "M", "M", 2, lastRow)},
				{Name: "Notes Added", Categories: rangeRef("Summary", "A", "A", 2, lastRow), Values: rangeRef("Summary", "N", "N", 2, lastRow)},
			},
			Title:     excelize.ChartTitle{Paragraph: []excelize.RichTextRun{{Text: "Agent Activity Summary"}}},
			Legend:    excelize.ChartLegend{Position: "bottom"},
			Dimension: excelize.ChartDimension{Width: 560, Height: 320},
		})

		// Call Activity: stacked column of outcome breakdown.
		_ = f.AddChart("Call Activity", "I2", &excelize.Chart{
			Type: excelize.ColStacked,
			Series: []excelize.ChartSeries{
				{Name: "Connected", Categories: rangeRef("Call Activity", "A", "A", 2, lastRow), Values: rangeRef("Call Activity", "C", "C", 2, lastRow)},
				{Name: "Completed", Categories: rangeRef("Call Activity", "A", "A", 2, lastRow), Values: rangeRef("Call Activity", "D", "D", 2, lastRow)},
				{Name: "Unanswered", Categories: rangeRef("Call Activity", "A", "A", 2, lastRow), Values: rangeRef("Call Activity", "E", "E", 2, lastRow)},
				{Name: "Busy", Categories: rangeRef("Call Activity", "A", "A", 2, lastRow), Values: rangeRef("Call Activity", "F", "F", 2, lastRow)},
				{Name: "Failed", Categories: rangeRef("Call Activity", "A", "A", 2, lastRow), Values: rangeRef("Call Activity", "G", "G", 2, lastRow)},
			},
			Title:     excelize.ChartTitle{Paragraph: []excelize.RichTextRun{{Text: "Call Outcome Breakdown"}}},
			Legend:    excelize.ChartLegend{Position: "bottom"},
			Dimension: excelize.ChartDimension{Width: 560, Height: 320},
		})

		// Efficiency: column of Talk Time vs Idle Time (seconds).
		_ = f.AddChart("Efficiency", "K2", &excelize.Chart{
			Type: excelize.Col,
			Series: []excelize.ChartSeries{
				{Name: "Talk Time (s)", Categories: rangeRef("Efficiency", "A", "A", 2, lastRow), Values: rangeRef("Efficiency", "C", "C", 2, lastRow)},
				{Name: "Idle Time (s)", Categories: rangeRef("Efficiency", "A", "A", 2, lastRow), Values: rangeRef("Efficiency", "G", "G", 2, lastRow)},
			},
			Title:     excelize.ChartTitle{Paragraph: []excelize.RichTextRun{{Text: "Talk Time vs Idle Time"}}},
			Legend:    excelize.ChartLegend{Position: "bottom"},
			Dimension: excelize.ChartDimension{Width: 560, Height: 320},
		})

		// Outcomes: doughnut of total appointments/conversions/status updates/notes.
		_ = f.AddChart("Outcomes", "G7", &excelize.Chart{
			Type: excelize.Doughnut,
			Series: []excelize.ChartSeries{
				{Name: "Outcomes", Categories: "'Outcomes'!$G$2:$G$5", Values: "'Outcomes'!$H$2:$H$5"},
			},
			Title:     excelize.ChartTitle{Paragraph: []excelize.RichTextRun{{Text: "Total Outcomes"}}},
			Legend:    excelize.ChartLegend{Position: "right"},
			Dimension: excelize.ChartDimension{Width: 480, Height: 320},
			PlotArea:  excelize.ChartPlotArea{ShowPercent: true, ShowCatName: false, ShowSerName: false, ShowVal: false},
		})
	}

	// Remove default Sheet1
	_ = f.DeleteSheet("Sheet1")

	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=agent_report_%s_to_%s.xlsx", fromStr, toStr))
	if _, err := f.WriteTo(w); err != nil {
		s.logger.Sugar().Errorw("agentReportXLSX", "writeErr", err)
	}
}
