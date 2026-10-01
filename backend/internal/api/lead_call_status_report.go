package api

import (
	"encoding/csv"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/globussoft/callified-backend/internal/db"
	"github.com/xuri/excelize/v2"
)

type leadCallStatusOptions struct {
	Agents       []reportUserOption      `json:"agents"`
	Dispositions []db.ReportFilterOption `json:"dispositions"`
}

type reportUserOption struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

func parseReportDate(value string, endOfDay bool) (*time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	parsed, err := time.Parse("2006-01-02", value)
	if err != nil {
		return nil, fmt.Errorf("date must use YYYY-MM-DD")
	}
	if endOfDay {
		parsed = parsed.AddDate(0, 0, 1)
	}
	return &parsed, nil
}

func parseReportStrings(value string) []string {
	seen := make(map[string]bool)
	values := make([]string, 0)
	for _, part := range strings.Split(value, ",") {
		part = strings.ToLower(strings.TrimSpace(part))
		if part == "" || seen[part] {
			continue
		}
		seen[part] = true
		values = append(values, part)
	}
	return values
}

func reportPageValue(value string, fallback, maximum int) int {
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 1 {
		return fallback
	}
	if parsed > maximum {
		return maximum
	}
	return parsed
}

func (s *Server) leadCallStatusUserScope(ac AuthClaims, requested []int64) ([]int64, error) {
	if ac.Role == db.RoleAdmin || ac.Role == "SuperAdmin" {
		return requested, nil
	}

	allowed := []int64{ac.UserID}
	if ac.Role == db.RoleTeamLeader {
		managed, err := s.db.GetManagedUserIDs(ac.UserID)
		if err != nil {
			return nil, err
		}
		allowed = append(allowed, managed...)
	}
	if len(requested) == 0 {
		return allowed, nil
	}
	allowedSet := make(map[int64]bool, len(allowed))
	for _, id := range allowed {
		allowedSet[id] = true
	}
	result := make([]int64, 0, len(requested))
	for _, id := range requested {
		if allowedSet[id] {
			result = append(result, id)
		}
	}
	if len(result) == 0 {
		return []int64{-1}, nil
	}
	return result, nil
}

func (s *Server) parseLeadCallStatusFilter(r *http.Request, ac AuthClaims) (db.LeadCallStatusFilter, error) {
	query := r.URL.Query()
	from, err := parseReportDate(query.Get("from"), false)
	if err != nil {
		return db.LeadCallStatusFilter{}, fmt.Errorf("invalid from date: %w", err)
	}
	to, err := parseReportDate(query.Get("to"), true)
	if err != nil {
		return db.LeadCallStatusFilter{}, fmt.Errorf("invalid to date: %w", err)
	}
	if from != nil && to != nil && !from.Before(*to) {
		return db.LeadCallStatusFilter{}, fmt.Errorf("from date must not be after to date")
	}
	agents, err := s.leadCallStatusUserScope(ac, parseExecutiveIDs(query.Get("agent_ids")))
	if err != nil {
		return db.LeadCallStatusFilter{}, err
	}
	return db.LeadCallStatusFilter{
		From:         from,
		To:           to,
		CampaignIDs:  parseExecutiveIDs(query.Get("campaign_ids")),
		AgentUserIDs: agents,
		Dispositions: parseReportStrings(query.Get("dispositions")),
		Search:       strings.TrimSpace(query.Get("search")),
		Page:         reportPageValue(query.Get("page"), 1, 1000000),
		Limit:        reportPageValue(query.Get("limit"), 50, 200),
	}, nil
}

func (s *Server) leadCallStatusReport(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, "reports.view") {
		return
	}
	filter, err := s.parseLeadCallStatusFilter(r, getAuth(r))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	items, total, err := s.db.GetLeadCallStatusReport(getAuth(r).OrgID, filter)
	if err != nil {
		s.logger.Sugar().Errorw("leadCallStatusReport", "err", err)
		writeError(w, http.StatusInternalServerError, "could not load lead call status report")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items": items, "total": total, "page": filter.Page, "limit": filter.Limit,
	})
}

func (s *Server) leadCallStatusReportOptions(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, "reports.view") {
		return
	}
	ac := getAuth(r)
	var users []db.User
	var err error
	if ac.Role == db.RoleAdmin || ac.Role == "SuperAdmin" {
		users, err = s.db.GetUsersByOrg(ac.OrgID)
	} else if ac.Role == db.RoleTeamLeader {
		users, err = s.db.GetAgentsByManager(ac.UserID)
		if self, selfErr := s.db.GetUserByIDInOrgWithRole(ac.UserID, ac.OrgID); selfErr == nil && self != nil {
			users = append([]db.User{*self}, users...)
		}
	} else {
		if self, selfErr := s.db.GetUserByIDInOrgWithRole(ac.UserID, ac.OrgID); selfErr == nil && self != nil {
			users = []db.User{*self}
		} else {
			err = selfErr
		}
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load report filters")
		return
	}
	agents := make([]reportUserOption, 0, len(users))
	for _, user := range users {
		if !user.IsActive {
			continue
		}
		name := strings.TrimSpace(user.FullName)
		if name == "" {
			name = user.Email
		}
		agents = append(agents, reportUserOption{ID: user.ID, Name: name, Email: user.Email})
	}
	dispositions, err := s.db.GetLeadCallStatusDispositionOptions(ac.OrgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load report filters")
		return
	}
	writeJSON(w, http.StatusOK, leadCallStatusOptions{Agents: agents, Dispositions: dispositions})
}

func (s *Server) leadCallStatusHistory(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, "reports.view") {
		return
	}
	leadID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || leadID <= 0 {
		writeError(w, http.StatusBadRequest, "invalid lead id")
		return
	}
	filter, err := s.parseLeadCallStatusFilter(r, getAuth(r))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	filter.LeadID = leadID
	items, err := s.db.GetLeadCallStatusHistory(getAuth(r).OrgID, filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load lead call history")
		return
	}
	if len(items) == 0 {
		writeError(w, http.StatusNotFound, "no calls found for this lead")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func spreadsheetSafe(value string) string {
	trimmed := strings.TrimLeft(value, " \t\r\n")
	if trimmed != "" && strings.ContainsRune("=+-@", rune(trimmed[0])) {
		return "'" + value
	}
	return value
}

func reportDuration(value float64) string {
	seconds := int64(value + 0.5)
	return fmt.Sprintf("%02d:%02d:%02d", seconds/3600, (seconds%3600)/60, seconds%60)
}

func reportExportRows(items []db.LeadCallStatusRow) [][]string {
	rows := [][]string{{"Lead Name", "Phone", "Campaign", "Last Call Date", "Disposition", "AI Summary", "Agent", "Call Count", "Total Duration"}}
	for _, item := range items {
		rows = append(rows, []string{
			spreadsheetSafe(item.LeadName), spreadsheetSafe(item.Phone), spreadsheetSafe(item.CampaignName),
			item.LastCallDate, spreadsheetSafe(item.DispositionLabel), spreadsheetSafe(item.AISummary),
			spreadsheetSafe(item.AgentName), strconv.FormatInt(item.CallCount, 10), reportDuration(item.TotalDurationS),
		})
	}
	return rows
}

func (s *Server) leadCallStatusExport(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, "reports.download") {
		return
	}
	filter, err := s.parseLeadCallStatusFilter(r, getAuth(r))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	filter.Page, filter.Limit = 1, 100000
	items, _, err := s.db.GetLeadCallStatusReport(getAuth(r).OrgID, filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not export lead call status report")
		return
	}
	rows := reportExportRows(items)
	stamp := time.Now().Format("20060102")
	if strings.EqualFold(r.URL.Query().Get("format"), "xlsx") {
		s.writeLeadCallStatusXLSX(w, rows, stamp)
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="lead-call-status-%s.csv"`, stamp))
	_, _ = w.Write([]byte{0xEF, 0xBB, 0xBF})
	writer := csv.NewWriter(w)
	_ = writer.WriteAll(rows)
}

func (s *Server) writeLeadCallStatusXLSX(w http.ResponseWriter, rows [][]string, stamp string) {
	file := excelize.NewFile()
	defer file.Close()
	const sheet = "Lead Call Status"
	_ = file.SetSheetName("Sheet1", sheet)
	for rowIndex, row := range rows {
		for columnIndex, value := range row {
			cell, _ := excelize.CoordinatesToCellName(columnIndex+1, rowIndex+1)
			_ = file.SetCellStr(sheet, cell, value)
		}
	}
	headerStyle, _ := file.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true, Color: "FFFFFF"}, Fill: excelize.Fill{Type: "pattern", Color: []string{"4F46E5"}, Pattern: 1}})
	_ = file.SetCellStyle(sheet, "A1", "I1", headerStyle)
	_ = file.AutoFilter(sheet, fmt.Sprintf("A1:I%d", len(rows)), nil)
	_ = file.SetPanes(sheet, &excelize.Panes{Freeze: true, Split: false, YSplit: 1, TopLeftCell: "A2", ActivePane: "bottomLeft"})
	widths := []float64{24, 18, 24, 20, 20, 52, 24, 12, 16}
	for index, width := range widths {
		column, _ := excelize.ColumnNumberToName(index + 1)
		_ = file.SetColWidth(sheet, column, column, width)
	}
	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="lead-call-status-%s.xlsx"`, stamp))
	if err := file.Write(w); err != nil {
		s.logger.Sugar().Errorw("writeLeadCallStatusXLSX", "err", err)
	}
}
