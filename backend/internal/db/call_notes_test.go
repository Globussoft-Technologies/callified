package db

import (
	"database/sql"
	"encoding/json"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestNotesOnlyRecognizeManualConversations(t *testing.T) {
	for _, tc := range []struct {
		text   string
		manual bool
	}{
		{`[{"role":"agent","text":"Hello"},{"role":"User","text":"Hi"}]`, true},
		{`[{"role":"system","text":"Human call"}]`, true},
		{`[{"role":"AI","text":"Hello"},{"role":"User","text":"Hi"}]`, false},
		{`[{"role":"User","text":"Human call"}]`, false},
		{`[]`, false},
	} {
		require.Equal(t, tc.manual, (&Transcript{Transcript: json.RawMessage(tc.text)}).IsManualCall())
	}
}

func TestManualNotesAreScopedAndIndependentOfAnalysis(t *testing.T) {
	pool, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer pool.Close()
	d := &DB{pool: pool}
	mock.ExpectQuery("SELECT notes FROM manual_call_notes WHERE org_id=.*transcript_id=").WithArgs(int64(7), int64(9)).WillReturnError(sql.ErrNoRows)
	notes, err := d.GetManualCallNotes(7, 9)
	require.NoError(t, err)
	require.Empty(t, notes)
	mock.ExpectExec("INSERT INTO manual_call_notes").WithArgs(int64(7), int64(9), "Call back Tuesday", int64(3)).WillReturnResult(sqlmock.NewResult(1, 1))
	require.NoError(t, d.SaveManualCallNotes(7, 9, 3, "Call back Tuesday"))
	mock.ExpectQuery("SELECT notes FROM manual_call_notes WHERE org_id=.*transcript_id=").WithArgs(int64(7), int64(9)).WillReturnRows(sqlmock.NewRows([]string{"notes"}).AddRow("Call back Tuesday"))
	notes, err = d.GetManualCallNotes(7, 9)
	require.NoError(t, err)
	require.Equal(t, "Call back Tuesday", notes)
	require.NoError(t, mock.ExpectationsWereMet())
}
