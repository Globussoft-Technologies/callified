package api

import (
	"bytes"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/globussoft/callified-backend/internal/db"
	"github.com/stretchr/testify/require"
	"github.com/xuri/excelize/v2"
)

type userExportFixture struct {
	t       *testing.T
	offsets []int64
}

func (f *userExportFixture) GetUserAssignedLeads(orgID, userID, limit, offset, campaignID int64) ([]db.UserAssignedLead, error) {
	require.EqualValues(f.t, 7, orgID)
	require.EqualValues(f.t, 9, userID)
	require.EqualValues(f.t, 1000, limit)
	require.EqualValues(f.t, 3, campaignID)
	f.offsets = append(f.offsets, offset)
	count := 1000
	if offset > 0 {
		count = 1
	}
	rows := make([]db.UserAssignedLead, count)
	for i := range rows {
		rows[i].FirstName = "Selected user's lead"
		rows[i].ID = offset + int64(i) + 1
	}
	return rows, nil
}
func (f *userExportFixture) GetUserRecordings(orgID, userID int64, from, to time.Time, limit, offset int64) ([]db.UserRecording, int64, error) {
	require.EqualValues(f.t, 7, orgID)
	require.EqualValues(f.t, 9, userID)
	return []db.UserRecording{{LeadName: "Recorded lead", DurationS: 12.5}}, 1, nil
}
func TestUserDetailDownloadIncludesAllPagesAndSafeProfile(t *testing.T) {
	store := &userExportFixture{t: t}
	response := httptest.NewRecorder()
	data := db.UserDetailResponse{Profile: db.User{ID: 9, OrgID: 7, FullName: "Selected Admin", Email: "admin@example.com", Role: "Admin", PasswordHash: "must-not-export"}}
	err := writeUserDetailExport(response, httptest.NewRequest("GET", "/", nil), data, time.Time{}, time.Time{}, 3, store, "https://testgo2.callified.ai")
	require.NoError(t, err)
	require.Equal(t, []int64{0, 1000}, store.offsets)
	workbook, err := excelize.OpenReader(bytes.NewReader(response.Body.Bytes()))
	require.NoError(t, err)
	defer workbook.Close()
	require.Equal(t, []string{"Profile", "Overview", "Campaigns", "Leads", "Recordings", "Calls"}, workbook.GetSheetList())
	leads, err := workbook.GetRows("Leads")
	require.NoError(t, err)
	require.Len(t, leads, 1002)
	profile, err := workbook.GetRows("Profile")
	require.NoError(t, err)
	require.Equal(t, "Selected Admin", profile[1][0])
	require.NotContains(t, profile[1], "must-not-export")
	recordings, err := workbook.GetRows("Recordings")
	require.NoError(t, err)
	require.Len(t, recordings, 2)
	calls, err := workbook.GetRows("Calls")
	require.NoError(t, err)
	require.Len(t, calls, 3)
	require.Equal(t, "Customer One", calls[1][4])
	require.Equal(t, "2026-11-16 12:30:00", calls[1][7])
	require.Equal(t, "Customer asked for a quote.\nCall back tomorrow.", calls[1][10])
	hasLink, target, err := workbook.GetCellHyperLink("Calls", "L2")
	require.NoError(t, err)
	require.True(t, hasLink)
	require.Contains(t, target, "/call-recording?src=")
	require.Equal(t, "Summary not available", calls[2][10])
	require.Equal(t, "Not available", calls[2][11])
	require.Contains(t, response.Header().Get("Content-Disposition"), "user_9_details.xlsx")
}

func (f *userExportFixture) GetUserExportCalls(orgID, userID int64, from, to time.Time, campaignID, limit, offset int64) ([]db.UserExportCall, error) {
	require.EqualValues(f.t, 7, orgID)
	require.EqualValues(f.t, 9, userID)
	require.EqualValues(f.t, 3, campaignID)
	return []db.UserExportCall{
		{ID: 99, CallSID: "call-99", CallerName: "Selected Admin", LeadName: "Customer One", CalledAt: "2026-11-16 12:30:00", Summary: "Customer asked for a quote. Call back tomorrow. Third sentence must be omitted.", RecordingURL: "/api/recordings/call.wav"},
		{ID: 100, LeadName: "Unanswered Customer", Status: "no-answer"},
	}, nil
}

func TestExportSummaryAndRecordingLink(t *testing.T) {
	require.Equal(t, "First sentence.\nSecond sentence.", shortExportSummary("First sentence. Second sentence. Third sentence."))
	require.Equal(t, "Summary not available", shortExportSummary(""))
	link, err := recordingExportLink("https://testgo2.callified.ai", "/api/recordings/agent/call.wav")
	require.NoError(t, err)
	require.Equal(t, "https://testgo2.callified.ai/call-recording?src=%2Fapi%2Frecordings%2Fagent%2Fcall.wav", link)
}
