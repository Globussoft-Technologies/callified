package wshandler

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestLiveAudioPlayoutFramesCompletedTurn(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sess := NewCallSession("test", nil, zap.NewNop())
	sess.UseUlaw = true
	p := newLiveAudioPlayout(ctx, sess)

	var mu sync.Mutex
	var frames [][]byte
	p.send = func(_ uint64, frame []byte) {
		mu.Lock()
		frames = append(frames, append([]byte(nil), frame...))
		mu.Unlock()
	}

	// 60 ms of 8 kHz PCM becomes three 20 ms μ-law carrier frames.
	require.True(t, p.QueuePCM(sess.PlaybackEpoch(), make([]byte, 480*2)))
	require.True(t, p.EndTurn(sess.PlaybackEpoch()))
	require.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(frames) == 3
	}, time.Second, 5*time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	for _, frame := range frames {
		require.Len(t, frame, liveUlawFrameBytes)
	}
}
