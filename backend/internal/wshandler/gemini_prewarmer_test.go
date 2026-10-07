package wshandler

import (
	"context"
	"sync"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/globussoft/callified-backend/internal/realtime"
)

func TestPreparedGeminiCallbacksReplayInOrder(t *testing.T) {
	sink := newPreparedGeminiCallbacks()
	cb := sink.callbacks(zap.NewNop(), 1)
	cb.OnOutputTranscript("hello")
	cb.OnAudio([]byte{1, 2})
	cb.OnTurnComplete()
	var mu sync.Mutex
	var got []string
	sink.attach(realtime.Callbacks{
		OnOutputTranscript: func(text string) { mu.Lock(); got = append(got, text); mu.Unlock() },
		OnAudio:            func(pcm []byte) { mu.Lock(); got = append(got, "audio"); mu.Unlock() },
		OnTurnComplete:     func() { mu.Lock(); got = append(got, "complete"); mu.Unlock() },
	})
	cb.OnAudio([]byte{3})
	mu.Lock()
	defer mu.Unlock()
	if len(got) != 4 || got[0] != "hello" || got[1] != "audio" || got[2] != "complete" || got[3] != "audio" {
		t.Fatalf("callback order = %v", got)
	}
}

func TestPreparedGeminiCallLifetimeAfterAttach(t *testing.T) {
	prepared, liveCtx := newPreparedGeminiCall(nil, newPreparedGeminiCallbacks())
	defer prepared.cancel()
	if deadline, ok := liveCtx.Deadline(); ok {
		t.Fatalf("live connection has a pre-answer deadline: %v", deadline)
	}
	h := &Handler{log: zap.NewNop()}
	const callSID = "answered-call"
	h.preparedLive.Store(callSID, prepared)
	expired := make(chan struct{})
	prepared.timer = time.AfterFunc(100*time.Millisecond, func() {
		if h.preparedLive.CompareAndDelete(callSID, prepared) {
			prepared.cancel()
		}
		close(expired)
	})
	if got := h.takePreparedGeminiCall(callSID); got != prepared {
		t.Fatal("prepared connection was not transferred to the answered call")
	}
	select {
	case <-liveCtx.Done():
		t.Fatal("answered call was cancelled by the pre-answer lifetime")
	case <-expired:
		t.Fatal("pre-answer expiry timer was not stopped")
	case <-time.After(150 * time.Millisecond):
	}
	callCtx, endCall := context.WithCancel(context.Background())
	carrierReady := make(chan struct{})
	finished := make(chan error, 1)
	go func() {
		finished <- prepared.runAttached(callCtx, carrierReady, nil, func() {}, func() {})
	}()
	endCall()
	select {
	case err := <-finished:
		if err != context.Canceled {
			t.Fatalf("attached call returned %v, want context.Canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("attached call did not end on carrier cancellation")
	}
	select {
	case <-liveCtx.Done():
	default:
		t.Fatal("live connection was not cancelled when the answered call ended")
	}
}

func TestPreparedGeminiCallBuffersCustomerAudioUntilGreeting(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sink := newPreparedGeminiCallbacks()
	carrierReady := make(chan struct{})
	input := make(chan []byte, 1)
	input <- []byte{1, 2}
	prepared := &preparedGeminiCall{
		sink: sink, audioIn: make(chan []byte, 1), done: make(chan error, 1), cancel: cancel,
	}
	finished := make(chan error, 1)
	attached := make(chan struct{}, 1)
	go func() {
		finished <- prepared.runAttached(ctx, carrierReady, input, func() { attached <- struct{}{} }, func() {})
	}()
	close(sink.firstAudio)
	select {
	case <-prepared.audioIn:
		t.Fatal("customer audio reached Gemini before Tata media was ready")
	case <-time.After(30 * time.Millisecond):
	}
	select {
	case <-attached:
		t.Fatal("prepared greeting attached before Tata media was ready")
	default:
	}
	close(carrierReady)
	select {
	case <-attached:
	case <-time.After(time.Second):
		t.Fatal("prepared greeting was not attached after Tata media became ready")
	}
	select {
	case pcm := <-prepared.audioIn:
		if len(pcm) != 2 || pcm[0] != 1 || pcm[1] != 2 {
			t.Fatalf("forwarded PCM = %v", pcm)
		}
	case <-time.After(time.Second):
		t.Fatal("customer audio not forwarded after greeting")
	}
	prepared.done <- nil
	select {
	case err := <-finished:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("prepared session did not finish")
	}
}
