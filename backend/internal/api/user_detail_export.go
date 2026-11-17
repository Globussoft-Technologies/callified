package api

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/globussoft/callified-backend/internal/db"
	"github.com/xuri/excelize/v2"
)

type userDetailExportStore interface {
	GetUserExportCalls(orgID, userID int64, from, to time.Time, campaignID, limit, offset int64) ([]db.UserExportCall, error)
	GetUserAssignedLeads(orgID, userID, limit, offset, campaignID int64) ([]db.UserAssignedLead, error)
	GetUserRecordings(orgID, userID int64, from, to time.Time, limit, offset int64) ([]db.UserRecording, int64, error)
}

func writeUserDetailExport(w http.ResponseWriter, r *http.Request, data db.UserDetailResponse, from, to time.Time, campaignID int64, store userDetailExportStore, publicURL string) error {
	f := excelize.NewFile()
	defer f.Close()
	write := func(sheet string, row int, values []any) error {
		cell, err := excelize.CoordinatesToCellName(1, row)
		if err != nil {
			return err
		}
		return f.SetSheetRow(sheet, cell, &values)
	}
	newSheet := func(name string, headers []any) error {
		if _, err := f.NewSheet(name); err != nil {
			return err
		}
		return write(name, 1, headers)
	}
	if err := newSheet("Profile", []any{"Name", "Email", "Role", "From", "To"}); err != nil {
		return err
	}
	date := func(t time.Time) string {
		if t.IsZero() {
			return "All dates"
		}
		return t.Format("2006-01-02")
	}
	if err := write("Profile", 2, []any{data.Profile.FullName, data.Profile.Email, data.Profile.Role, date(from), date(to)}); err != nil {
		return err
	}
	leadLabel, campaignLabel := "Assigned Leads", "Assigned Campaigns"
	if data.Profile.Role == "Admin" {
		leadLabel, campaignLabel = "Organization Leads", "Organization Campaigns"
	}
	if err := newSheet("Overview", []any{leadLabel, campaignLabel, "Total Calls", "Recordings", "Appointments", "Notes"}); err != nil {
		return err
	}
	v := data.Stats
	if err := write("Overview", 2, []any{v.AssignedLeads, v.AssignedCampaigns, v.TotalCalls, v.Recordings, v.Appointments, v.Notes}); err != nil {
		return err
	}
	if err := newSheet("Campaigns", []any{"Campaign ID", "Name", "Status", "Leads", "Calls", "Recordings", "Appointments", "Notes"}); err != nil {
		return err
	}
	for i, c := range data.Campaigns {
		if err := write("Campaigns", i+2, []any{c.CampaignID, c.Name, c.Status, c.LeadCount, c.TotalCalls, c.Recordings, c.Appointments, c.Notes}); err != nil {
			return err
		}
	}
	if err := newSheet("Leads", []any{"Lead ID", "Name", "Phone", "Campaign", "Status", "Company", "Interest", "Follow-up Note", "Follow-up At"}); err != nil {
		return err
	}
	row := 2
	for offset := int64(0); ; offset += 1000 {
		if err := r.Context().Err(); err != nil {
			return err
		}
		leads, err := store.GetUserAssignedLeads(data.Profile.OrgID, data.Profile.ID, 1000, offset, campaignID)
		if err != nil {
			return err
		}
		for _, l := range leads {
			if err := write("Leads", row, []any{l.ID, l.FirstName + " " + l.LastName, l.Phone, l.CampaignName, l.Status, l.Company, l.Interest, l.FollowUpNote, l.FollowUpAt}); err != nil {
				return err
			}
			row++
		}
		if len(leads) < 1000 {
			break
		}
	}
	if err := newSheet("Recordings", []any{"Call ID", "Call SID", "Lead", "Phone", "Campaign", "Provider", "Status", "Duration (seconds)", "Recording URL", "Call At"}); err != nil {
		return err
	}
	row = 2
	for offset := int64(0); ; offset += 1000 {
		if err := r.Context().Err(); err != nil {
			return err
		}
		recordings, _, err := store.GetUserRecordings(data.Profile.OrgID, data.Profile.ID, from, to, 1000, offset)
		if err != nil {
			return err
		}
		for _, c := range recordings {
			if err := write("Recordings", row, []any{c.ID, c.CallSid, c.LeadName, c.Phone, c.CampaignName, c.Provider, c.Status, c.DurationS, c.RecordingURL, c.UserCallAt}); err != nil {
				return err
			}
			if c.RecordingURL != "" {
				link, err := recordingExportLink(publicURL, c.RecordingURL)
				if err != nil {
					return err
				}
				if err := f.SetCellHyperLink("Recordings", fmt.Sprintf("I%d", row), link, "External"); err != nil {
					return err
				}
			}
			row++
		}
		if len(recordings) < 1000 {
			break
		}
	}
	if err := newSheet("Calls", []any{"Call ID", "Call SID", "Caller", "Caller Email", "Lead", "Phone", "Campaign", "Call Time (UTC)", "Status", "Duration (seconds)", "Short Summary", "Recording"}); err != nil {
		return err
	}
	row = 2
	for offset := int64(0); ; offset += 1000 {
		if err := r.Context().Err(); err != nil {
			return err
		}
		calls, err := store.GetUserExportCalls(data.Profile.OrgID, data.Profile.ID, from, to, campaignID, 1000, offset)
		if err != nil {
			return err
		}
		for _, c := range calls {
			summary := shortExportSummary(c.Summary)
			recording := "Not available"
			if c.RecordingURL != "" {
				recording = "Open recording"
			}
			if err := write("Calls", row, []any{c.ID, c.CallSID, c.CallerName, c.CallerEmail, c.LeadName, c.Phone, c.CampaignName, c.CalledAt, c.Status, c.DurationS, summary, recording}); err != nil {
				return err
			}
			if c.RecordingURL != "" {
				link, err := recordingExportLink(publicURL, c.RecordingURL)
				if err != nil {
					return err
				}
				cell := fmt.Sprintf("L%d", row)
				if err := f.SetCellHyperLink("Calls", cell, link, "External"); err != nil {
					return err
				}
			}
			row++
		}
		if len(calls) < 1000 {
			break
		}
	}
	style, err := f.NewStyle(&excelize.Style{Alignment: &excelize.Alignment{WrapText: true, Vertical: "top"}})
	if err != nil {
		return err
	}
	if err := f.SetColWidth("Calls", "K", "K", 75); err != nil {
		return err
	}
	if err := f.SetColStyle("Calls", "K", style); err != nil {
		return err
	}
	for _, sheet := range []string{"Calls", "Recordings"} {
		_ = f.SetPanes(sheet, &excelize.Panes{Freeze: true, YSplit: 1, TopLeftCell: "A2", ActivePane: "bottomLeft"})
	}

	_ = f.DeleteSheet("Sheet1")
	if r.URL.Query().Get("format") == "pdf" {
		return writeReportPDF(w, r, f, fmt.Sprintf("user_%d_details", data.Profile.ID))
	}
	buffer, err := f.WriteToBuffer()
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="user_%d_details.xlsx"`, data.Profile.ID))
	_, err = w.Write(buffer.Bytes())
	return err
}

func recordingExportLink(publicURL, recording string) (string, error) {
	base, err := url.Parse(publicURL)
	if err != nil || base.Host == "" || (base.Scheme != "https" && base.Scheme != "http") {
		return "", fmt.Errorf("public server URL is not configured for recording links")
	}
	base.Path = "/call-recording"
	base.RawQuery = url.Values{"src": {recording}}.Encode()
	base.Fragment = ""
	return base.String(), nil
}

func shortExportSummary(summary string) string {
	summary = strings.Join(strings.Fields(summary), " ")
	if summary == "" {
		return "Summary not available"
	}
	lines := []string{}
	start := 0
	for i, r := range summary {
		if r == '.' || r == '!' || r == '?' || r == '।' {
			end := i + utf8.RuneLen(r)
			if end == len(summary) || summary[end] == ' ' {
				lines = append(lines, strings.TrimSpace(summary[start:end]))
				start = end
				if len(lines) == 2 {
					break
				}
			}
		}
	}
	if len(lines) < 2 && strings.TrimSpace(summary[start:]) != "" {
		lines = append(lines, strings.TrimSpace(summary[start:]))
	}
	if len(lines) > 2 {
		lines = lines[:2]
	}
	for i, line := range lines {
		runes := []rune(line)
		if len(runes) > 180 {
			lines[i] = string(runes[:177]) + "…"
		}
	}
	return strings.Join(lines, "\n")
}
