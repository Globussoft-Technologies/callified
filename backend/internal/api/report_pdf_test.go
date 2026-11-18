package api

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/xuri/excelize/v2"
)

func TestReportPDFIncludesAllRecordsAndRecordingLinks(t *testing.T) {
	f := excelize.NewFile()
	defer f.Close()
	require.NoError(t, f.SetSheetRow("Sheet1", "A1", &[]any{"Lead", "Recording", "Summary"}))
	for row := 2; row < 42; row++ {
		cell, err := excelize.CoordinatesToCellName(1, row)
		require.NoError(t, err)
		require.NoError(t, f.SetSheetRow("Sheet1", cell, &[]any{"Customer", "Listen", strings.Repeat("Summary text. ", 20)}))
	}
	require.NoError(t, f.SetCellHyperLink("Sheet1", "B2", "https://example.com/call-recording?src=test", "External"))
	w := httptest.NewRecorder()
	require.NoError(t, writeReportPDF(w, httptest.NewRequest("GET", "/?format=pdf", nil), f, "report"))
	require.Equal(t, "application/pdf", w.Header().Get("Content-Type"))
	require.Contains(t, w.Header().Get("Content-Disposition"), "report.pdf")
	require.True(t, strings.HasPrefix(w.Body.String(), "%PDF-"))
	require.Contains(t, w.Body.String(), "https://example.com/call-recording?src=test")
	require.Greater(t, strings.Count(w.Body.String(), "/Type /Page\n"), 1)
}
