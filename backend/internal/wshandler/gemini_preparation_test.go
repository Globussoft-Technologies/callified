package wshandler

import (
	"context"
	"testing"
	"time"
)

func TestGeminiPreparationDoesNotBlockAndDiscardsLateResult(t *testing.T) {
	release := make(chan struct{})
	started := make(chan struct{})
	liveCtx, cancelLive := context.WithCancel(context.Background())
	defer cancelLive()
	pending := beginGeminiPreparation(context.Background(), time.Second, func(context.Context) *preparedGeminiCall {
		close(started)
		<-release // Simulates a legacy DB call that ignores cancellation.
		return &preparedGeminiCall{cancel: cancelLive}
	})
	<-started
	if got := pending.take(); got != nil {
		t.Fatal("unfinished preparation must not block call startup")
	}
	close(release)
	select {
	case <-liveCtx.Done():
	case <-time.After(time.Second):
		t.Fatal("late Gemini session leaked after fallback")
	}
}

func TestGeminiPreparationCancelsOnFailedDial(t *testing.T) {
	stopped := make(chan struct{})
	pending := beginGeminiPreparation(context.Background(), time.Second, func(ctx context.Context) *preparedGeminiCall {
		<-ctx.Done()
		close(stopped)
		return nil
	})
	pending.stop()
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("failed dial did not cancel preparation")
	}
}

func TestGeminiPreparationHonorsStartupBudget(t *testing.T) {
	stopped := make(chan struct{})
	pending := beginGeminiPreparation(context.Background(), 20*time.Millisecond, func(ctx context.Context) *preparedGeminiCall {
		<-ctx.Done()
		close(stopped)
		return nil
	})
	defer pending.stop()
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("preparation ignored startup deadline")
	}
}

func TestGeminiPreparationCancelsWithUnboundRequest(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	stopped := make(chan struct{})
	pending := beginGeminiPreparation(ctx, time.Second, func(ctx context.Context) *preparedGeminiCall {
		<-ctx.Done()
		close(stopped)
		return nil
	})
	defer pending.stop()
	cancel()
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("request cancellation did not reach preparation")
	}
}

func TestBoundGeminiPreparationSurvivesRequestCompletion(t *testing.T) {
	parent, cancel := context.WithCancel(context.Background())
	defer cancel()
	release := make(chan struct{})
	preparedReady := make(chan struct{})
	liveCtx, cancelLive := context.WithCancel(context.Background())
	defer cancelLive()
	pending := beginGeminiPreparation(parent, time.Second, func(ctx context.Context) *preparedGeminiCall {
		<-release
		if ctx.Err() != nil {
			t.Error("successful dial lost preparation on request completion")
		}
		close(preparedReady)
		return &preparedGeminiCall{cancel: cancelLive}
	})
	defer pending.stop()
	pending.mu.Lock()
	pending.bound = true
	pending.mu.Unlock()
	cancel()
	close(release)
	<-preparedReady
	// Wait for publication under the same lock, without assuming goroutine timing.
	deadline := time.Now().Add(time.Second)
	for {
		pending.mu.Lock()
		ready := pending.prepared != nil
		pending.mu.Unlock()
		if ready {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("prepared session was not published")
		}
		time.Sleep(time.Millisecond)
	}
	if pending.take() == nil {
		t.Fatal("ready session was not transferred")
	}
	select {
	case <-liveCtx.Done():
		t.Fatal("transferred session was cancelled")
	default:
	}
}
