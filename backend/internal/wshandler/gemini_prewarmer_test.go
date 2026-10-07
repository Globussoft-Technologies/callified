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

func TestPreparedGeminiPreAnswerTimeoutDoesNotEndAttachedCall(t *testing.T) {
	liveCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	prepared := &preparedGeminiCall{cancel: cancel}
	prepared.preAnswerTimer = time.AfterFunc(20*time.Millisecond, prepared.cancelIfUnattached)
	prepared.markAttached()

	// Even if a timeout callback was already queued when the call attached,
	// it must not cancel the active Live session.
	prepared.cancelIfUnattached()
	select {
	case <-liveCtx.Done():
		t.Fatal("attached Gemini Live session was cancelled by pre-answer timeout")
	case <-time.After(50 * time.Millisecond):
	}
}

func TestPreparedGeminiPreAnswerTimeoutCancelsUnattachedCall(t *testing.T) {
	liveCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	prepared := &preparedGeminiCall{cancel: cancel}
	prepared.preAnswerTimer = time.AfterFunc(20*time.Millisecond, prepared.cancelIfUnattached)

	select {
	case <-liveCtx.Done():
	case <-time.After(time.Second):
		t.Fatal("unattached Gemini Live session was not cancelled")
	}
}
