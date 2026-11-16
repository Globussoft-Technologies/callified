package wshandler

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestNormalCloseResumesOnlyForCustomerContinuation(t *testing.T) {
	for _, text := range []string{"Wait, I have one more question", "What is the price?", "Can you explain that again?", "మీ పేరు ఏంటి?", "रुकिए, कीमत क्या है?", "ಹೇಗೆ ಬಳಸುವುದು?"} {
		t.Run(text, func(t *testing.T) {
			s := NewCallSession("test", nil, zap.NewNop())
			s.RequestFinalClose()
			epoch := s.PlaybackEpoch()
			require.True(t, s.ResumeNormalClose(text))
			require.False(t, s.IsFinalClosing())
			require.False(t, s.HangupRequested())
			require.Greater(t, s.PlaybackEpoch(), epoch)
			require.False(t, s.CommitHangup(epoch), "old goodbye must not hang up a resumed call")
		})
	}
	for _, text := range []string{"Yes", "Okay", "Thanks", "Goodbye", "mhm", "whatever", "Thanks for showing me"} {
		s := NewCallSession("test", nil, zap.NewNop())
		s.RequestFinalClose()
		require.False(t, s.ResumeNormalClose(text), text)
		require.True(t, s.IsFinalClosing())
	}
}

func TestCommittedAndMaximumDurationClosingsCannotReopen(t *testing.T) {
	s := NewCallSession("test", nil, zap.NewNop())
	s.RequestFinalClose()
	require.True(t, s.CommitHangup(s.PlaybackEpoch()))
	require.False(t, s.ResumeNormalClose("Wait, what is your name?"))
	s = NewCallSession("test", nil, zap.NewNop())
	s.RequestFinalClose()
	s.RequestMaxDurationClose()
	require.False(t, s.ResumeNormalClose("Wait"))
	require.True(t, s.CommitHangup(s.PlaybackEpoch()+1), "hard duration limit must not be bypassed by an old playback epoch")
}

func TestTTSWorkerSurvivesCancelledGoodbyeAndConsumesMoreSpeech(t *testing.T) {
	s := NewCallSession("test", nil, zap.NewNop())
	s.RequestFinalClose()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() { runTTSWorker(ctx, s, nil); close(done) }()
	s.TTSSentences <- ""
	time.Sleep(30 * time.Millisecond)
	require.True(t, s.ResumeNormalClose("Wait, what is your name?"))
	s.TTSSentences <- "Here is the answer."
	require.Eventually(t, func() bool { return len(s.TTSSentences) == 0 }, time.Second, 10*time.Millisecond)
	select {
	case <-done:
		t.Fatal("worker exited after a cancelled goodbye")
	default:
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("worker did not shut down")
	}
}
