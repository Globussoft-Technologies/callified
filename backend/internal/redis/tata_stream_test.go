package redis

import (
	"context"
	"github.com/alicebob/miniredis/v2"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func streamTestStore(t *testing.T) (*Store, *miniredis.Miniredis) {
	m := miniredis.RunT(t)
	return New("redis://"+m.Addr(), zap.NewNop()), m
}
func inboundTestContext() TataStreamContext {
	return TataStreamContext{CallID: "call-a", AccountID: 2, Inbound: true, Pending: PendingCallInfo{OrgID: 1}}
}
func TestTataTicketRetriesReuseUnconsumedTicket(t *testing.T) {
	s, _ := streamTestStore(t)
	ctx := context.Background()
	info := inboundTestContext()
	first, err := s.IssueTataStreamTicket(ctx, info)
	require.NoError(t, err)
	retry, err := s.IssueTataStreamTicket(ctx, info)
	require.NoError(t, err)
	require.Equal(t, first, retry)
	_, err = s.ConsumeTataStreamTicket(ctx, first)
	require.NoError(t, err)
	_, err = s.IssueTataStreamTicket(ctx, info)
	require.Error(t, err)
}
func TestTataTicketSingleUseConcurrentAndExpiry(t *testing.T) {
	s, m := streamTestStore(t)
	ctx := context.Background()
	info := inboundTestContext()
	token, err := s.IssueTataStreamTicket(ctx, info)
	require.NoError(t, err)
	var n atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := s.ConsumeTataStreamTicket(ctx, token)
			if err == nil {
				n.Add(1)
			}
		}()
	}
	wg.Wait()
	require.Equal(t, int32(1), n.Load())
	info.CallID = "call-b"
	token, err = s.IssueTataStreamTicket(ctx, info)
	require.NoError(t, err)
	m.FastForward(46 * time.Second)
	_, err = s.ConsumeTataStreamTicket(ctx, token)
	require.Error(t, err)
	_, err = (&Store{}).IssueTataStreamTicket(ctx, info)
	require.Error(t, err)
}
func TestTataOutboundSetupRetainsContextAndRetries(t *testing.T) {
	s, m := streamTestStore(t)
	ctx := context.Background()
	pending := PendingCallInfo{OrgID: 1, ExotelAccountID: 2, Phone: "7795740488"}
	require.NoError(t, s.ReserveTataDial(ctx, &pending))
	got, err := s.GetTataDial(ctx, 2, "+917795740488")
	require.NoError(t, err)
	require.Equal(t, pending, got)
	// A failed ticket script must not delete the reservation.
	m.SetError("temporary Redis failure")
	info := TataStreamContext{CallID: "call-a", AccountID: 2, Pending: got}
	_, err = s.IssueTataStreamTicket(ctx, info)
	require.Error(t, err)
	m.SetError("")
	_, err = s.GetTataDial(ctx, 2, pending.Phone)
	require.NoError(t, err)
	token, err := s.IssueTataStreamTicket(ctx, info)
	require.NoError(t, err)
	retry, err := s.IssueTataStreamTicket(ctx, info)
	require.NoError(t, err)
	require.Equal(t, token, retry)
	wrong := info
	wrong.CallID = "different-call"
	_, err = s.IssueTataStreamTicket(ctx, wrong)
	require.Error(t, err)
	_, err = s.ConsumeTataStreamTicket(ctx, token)
	require.NoError(t, err)
	_, err = s.GetTataDial(ctx, 2, pending.Phone)
	require.Error(t, err)
}
func TestTataOldUpdateAndReleaseCannotTouchNewReservation(t *testing.T) {
	s, _ := streamTestStore(t)
	ctx := context.Background()
	old := PendingCallInfo{OrgID: 1, ExotelAccountID: 2, Phone: "7795740488", ExotelCallSid: "old"}
	require.NoError(t, s.ReserveTataDial(ctx, &old))
	s.ReleaseTataDial(ctx, old)
	current := old
	current.TataReservationID = ""
	current.ExotelCallSid = "new"
	require.NoError(t, s.ReserveTataDial(ctx, &current))
	require.NotEqual(t, old.TataReservationID, current.TataReservationID)
	s.UpdateTataDial(ctx, old)
	s.ReleaseTataDial(ctx, old)
	require.NoError(t, s.ReleaseTataDialForCall(ctx, old, "old"))
	got, err := s.GetTataDial(ctx, 2, current.Phone)
	require.NoError(t, err)
	require.Equal(t, "new", got.ExotelCallSid)
	require.NoError(t, s.ReleaseTataDialForCall(ctx, current, "new"))
	_, err = s.GetTataDial(ctx, 2, current.Phone)
	require.Error(t, err)
}
func TestTataAccountReservationsAreIsolated(t *testing.T) {
	s, _ := streamTestStore(t)
	ctx := context.Background()
	a := PendingCallInfo{OrgID: 1, ExotelAccountID: 2, Phone: "7795740488"}
	b := a
	b.OrgID = 3
	b.ExotelAccountID = 4
	require.NoError(t, s.ReserveTataDial(ctx, &a))
	require.NoError(t, s.ReserveTataDial(ctx, &b))
	duplicate := PendingCallInfo{OrgID: 1, ExotelAccountID: 2, Phone: "+917795740488"}
	require.Error(t, s.ReserveTataDial(ctx, &duplicate))
	got, err := s.GetTataDial(ctx, 4, b.Phone)
	require.NoError(t, err)
	require.Equal(t, int64(3), got.OrgID)
}
