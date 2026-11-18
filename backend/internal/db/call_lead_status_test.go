package db

import (
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestCompleteCallingLeadScopesLatestCall(t *testing.T) {
	pool, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer pool.Close()
	d := &DB{pool: pool}
	mock.ExpectExec("UPDATE leads SET status=.*SELECT MAX\\(id\\).*SELECT MAX\\(id\\)").
		WithArgs("Follow-up", int64(7), int64(9), "Follow-up", int64(7), int64(9), int64(7), int64(9), "call-1").WillReturnResult(sqlmock.NewResult(0, 1))
	require.NoError(t, d.CompleteCallingLead(7, 9, "call-1", "Follow-up"))
	require.NoError(t, d.CompleteCallingLead(0, 9, "call-1", "Follow-up"))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestLeadStatusForCallDisposition(t *testing.T) {
	for code, expected := range map[string]string{"interested": "Interested", "not_interested": "Not Interested", "callback": "Follow-up", "appointment_booked": "Appointment Set", "no_answer": "", "custom": "Contacted"} {
		require.Equal(t, expected, leadStatusForCallDisposition(code))
	}
}

func TestUnansweredAttemptDoesNotResetLeadStatus(t *testing.T) {
	pool, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer pool.Close()
	d := &DB{pool: pool}
	// No UPDATE is allowed for an unanswered attempt, regardless of prior status.
	require.NoError(t, d.CompleteCallingLead(7, 9, "unanswered-call", leadStatusForCallDisposition("no_answer")))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestBusyAttemptKeepsHistoryWithoutChangingBusinessStatus(t *testing.T) {
	pool, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer pool.Close()
	d := &DB{pool: pool}
	mock.ExpectQuery("SELECT id, org_id,.*FROM call_logs WHERE call_sid=").WithArgs("busy-call").
		WillReturnRows(sqlmock.NewRows([]string{"id", "org", "campaign", "lead", "agent"}).AddRow(10, 7, 8, 9, 3))
	mock.ExpectExec("INSERT INTO call_dispositions").WillReturnResult(sqlmock.NewResult(1, 1))
	// A third query here would mean the master lead was being overwritten.
	code, summary, ok := SystemDispositionForCallStatus("busy")
	require.True(t, ok)
	require.NoError(t, d.SaveSystemDispositionByCallSid("busy-call", code, summary))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRegeneratedAIUsesHumanReviewedDispositionForLeadStatus(t *testing.T) {
	pool, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer pool.Close()
	d := &DB{pool: pool}
	mock.ExpectExec("INSERT INTO call_dispositions").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectQuery("SELECT COALESCE.*final_disposition_code.*ai_disposition_code").
		WithArgs(int64(7), int64(9), "latest-call").
		WillReturnRows(sqlmock.NewRows([]string{"code"}).AddRow("not_interested"))
	mock.ExpectExec("UPDATE leads SET status=").
		WithArgs("Not Interested", int64(7), int64(9), "Not Interested", int64(7), int64(9), int64(7), int64(9), "latest-call").
		WillReturnResult(sqlmock.NewResult(0, 1))
	require.NoError(t, d.SaveAICallDisposition(AICallDisposition{
		OrgID: 7, LeadID: 9, CallSid: "latest-call", Code: "callback",
	}))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestCachedDispositionCanSynchronizeLeadWithoutNewAIRequest(t *testing.T) {
	pool, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer pool.Close()
	d := &DB{pool: pool}
	mock.ExpectExec("UPDATE leads SET status=").
		WithArgs("Not Interested", int64(7), int64(9), "Not Interested", int64(7), int64(9), int64(7), int64(9), "latest-call").
		WillReturnResult(sqlmock.NewResult(0, 1))
	require.NoError(t, d.SyncCallDispositionLeadStatus(&CallDisposition{
		OrgID: 7, LeadID: 9, CallSid: "latest-call", EffectiveCode: "not_interested",
	}))
	require.NoError(t, d.SyncCallDispositionLeadStatus(nil))
	require.NoError(t, mock.ExpectationsWereMet())
}
