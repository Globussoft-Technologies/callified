package wshandler

import (
	"context"
	"sync"
	"time"

	"github.com/globussoft/callified-backend/internal/dial"
)

// A reservation is published before preparation finishes. Answered calls claim
// it without waiting; late results are cancelled rather than becoming orphans.
type pendingGeminiPreparation struct {
	mu       sync.Mutex
	prepared *preparedGeminiCall
	cancel   context.CancelFunc
	claimed  bool
	bound    bool
	timer    *time.Timer
}

func (p *pendingGeminiPreparation) stop() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.claimed = true
	p.cancel()
	if p.timer != nil {
		p.timer.Stop()
	}
	if p.prepared != nil {
		p.prepared.cancel()
		p.prepared = nil
	}
}

func (p *pendingGeminiPreparation) take() *preparedGeminiCall {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.claimed = true
	p.cancel()
	if p.timer != nil {
		p.timer.Stop()
	}
	result := p.prepared
	p.prepared = nil
	return result
}

func beginGeminiPreparation(parent context.Context, timeout time.Duration, build func(context.Context) *preparedGeminiCall) *pendingGeminiPreparation {
	// A successful dial hands ownership to the answered-call lifecycle, not
	// the short-lived HTTP request. Cancellation before that handoff still wins.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(parent), timeout)
	p := &pendingGeminiPreparation{cancel: cancel}
	stopParent := context.AfterFunc(parent, func() {
		p.mu.Lock()
		bound := p.bound
		p.mu.Unlock()
		if !bound {
			p.stop()
		}
	})
	go func() {
		defer stopParent()
		prepared := build(ctx)
		p.mu.Lock()
		defer p.mu.Unlock()
		if p.claimed || ctx.Err() != nil {
			if prepared != nil {
				prepared.cancel()
			}
		} else {
			p.prepared = prepared
		}
		cancel()
	}()
	return p
}

func (h *Handler) prepareGeminiCall(ctx context.Context, data dial.CallData) func(string) {
	if h.promptBuilder == nil || data.IsBridge {
		return nil
	}
	pending := beginGeminiPreparation(ctx, 3*time.Second, func(ctx context.Context) *preparedGeminiCall {
		return h.buildPreparedGeminiCall(ctx, data)
	})
	return func(sid string) {
		if sid == "" {
			pending.stop()
			return
		}
		pending.mu.Lock()
		defer pending.mu.Unlock()
		if pending.claimed || ctx.Err() != nil {
			pending.cancel()
			if pending.prepared != nil {
				pending.prepared.cancel()
				pending.prepared = nil
			}
			return
		}
		pending.bound = true
		h.preparedLive.Store(sid, pending)
		pending.timer = time.AfterFunc(90*time.Second, func() {
			if h.preparedLive.CompareAndDelete(sid, pending) {
				pending.stop()
			}
		})
	}
}
