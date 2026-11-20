package workers

import (
	"context"
	"errors"
	"github.com/globussoft/callified-backend/internal/db"
	"github.com/globussoft/callified-backend/internal/dial"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"testing"
)

type retryTestStore struct {
	retryStore
	enabled               bool
	lookupErr             error
	leadLookups, attempts int
}

func (s *retryTestStore) GetPendingRetries() ([]db.Retry, error) {
	return []db.Retry{{ID: 1, LeadID: 2, CampaignID: 3, OrgID: 4}}, nil
}
func (s *retryTestStore) CampaignAutoRetriesEnabled(org, campaign int64) (bool, error) {
	if org != 4 || campaign != 3 {
		panic("incorrect campaign scope")
	}
	return s.enabled, s.lookupErr
}
func (s *retryTestStore) GetLeadByID(int64) (*db.Lead, error) {
	s.leadLookups++
	return &db.Lead{ID: 2}, nil
}
func (s *retryTestStore) GetCampaignVoiceSettings(int64) (db.VoiceSettings, error) {
	return db.VoiceSettings{}, nil
}
func (s *retryTestStore) IncrRetryAttempt(int64) (bool, error) { s.attempts++; return false, nil }

type retryTestDialer struct {
	calls int
	err   error
}

func (d *retryTestDialer) Initiate(context.Context, dial.CallData) (string, error) {
	d.calls++
	return "call", d.err
}
func TestRetryWorkerRechecksCampaignOptInBeforeDialingQueuedRetry(t *testing.T) {
	for _, tc := range []struct {
		name               string
		enabled            bool
		lookupErr, dialErr error
		calls, attempts    int
	}{
		{name: "disabled pauses queued work"},
		{name: "lookup fails closed", lookupErr: errors.New("database unavailable")},
		{name: "enabled resumes queued work", enabled: true, calls: 1, attempts: 1},
		{name: "credits still pause attempts", enabled: true, dialErr: dial.ErrInsufficientCredits, calls: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := &retryTestStore{enabled: tc.enabled, lookupErr: tc.lookupErr}
			dialer := &retryTestDialer{err: tc.dialErr}
			worker := &RetryWorker{db: store, initiator: dialer, log: zap.NewNop()}
			worker.tick(context.Background())
			require.Equal(t, tc.calls, dialer.calls)
			require.Equal(t, tc.attempts, store.attempts)
			if tc.calls == 0 {
				require.Zero(t, store.leadLookups)
			}
		})
	}
}
