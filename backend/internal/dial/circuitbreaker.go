package dial

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// CircuitState represents the three states of a circuit breaker.
type CircuitState int

const (
	// CircuitClosed allows requests through; failures are counted.
	CircuitClosed CircuitState = iota
	// CircuitOpen rejects requests immediately and fast-fails.
	CircuitOpen
	// CircuitHalfOpen allows a small number of probe requests through.
	CircuitHalfOpen
)

func (s CircuitState) String() string {
	switch s {
	case CircuitClosed:
		return "closed"
	case CircuitOpen:
		return "open"
	case CircuitHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

// CircuitBreakerConfig tunes the breaker behaviour.
type CircuitBreakerConfig struct {
	// FailureThreshold is the consecutive failures required to open the circuit.
	FailureThreshold int
	// SuccessThreshold is the consecutive successes required in half-open to close.
	SuccessThreshold int
	// OpenTimeout is how long the circuit stays open before becoming half-open.
	OpenTimeout time.Duration
}

// DefaultCircuitBreakerConfig is suitable for telephony providers.
func DefaultCircuitBreakerConfig() CircuitBreakerConfig {
	return CircuitBreakerConfig{
		FailureThreshold: 5,
		SuccessThreshold: 2,
		OpenTimeout:      30 * time.Second,
	}
}

// breaker tracks the state of one provider circuit.
type breaker struct {
	cfg             CircuitBreakerConfig
	state           CircuitState
	failures        int
	successes       int
	lastFailureTime time.Time
	openedAt        time.Time
	mu              sync.Mutex
}

func newBreaker(cfg CircuitBreakerConfig) *breaker {
	return &breaker{cfg: cfg, state: CircuitClosed}
}

// Registry holds a circuit breaker per provider name.
type Registry struct {
	cfg      CircuitBreakerConfig
	breakers map[string]*breaker
	mu       sync.RWMutex
}

// NewCircuitRegistry creates a new registry with the given config.
func NewCircuitRegistry(cfg CircuitBreakerConfig) *Registry {
	return &Registry{
		cfg:      cfg,
		breakers: make(map[string]*breaker),
	}
}

func (r *Registry) get(name string) *breaker {
	r.mu.Lock()
	defer r.mu.Unlock()
	b, ok := r.breakers[name]
	if !ok {
		b = newBreaker(r.cfg)
		r.breakers[name] = b
	}
	return b
}

// State returns the current state for a provider.
func (r *Registry) State(name string) CircuitState {
	return r.get(name).currentState()
}

// Allow returns nil if the request should proceed, otherwise returns an error
// indicating the circuit is open.
func (r *Registry) Allow(name string) error {
	b := r.get(name)
	if b.currentState() == CircuitOpen {
		return fmt.Errorf("circuit breaker open for provider %s (cooling down)", name)
	}
	return nil
}

// RecordSuccess marks a successful call for a provider.
func (r *Registry) RecordSuccess(name string) {
	r.get(name).recordSuccess()
}

// RecordFailure marks a failed call for a provider.
func (r *Registry) RecordFailure(name string) {
	r.get(name).recordFailure()
}

func (b *breaker) currentState() CircuitState {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.state == CircuitOpen && time.Since(b.openedAt) > b.cfg.OpenTimeout {
		b.state = CircuitHalfOpen
		b.successes = 0
	}
	return b.state
}

func (b *breaker) recordSuccess() {
	b.mu.Lock()
	defer b.mu.Unlock()
	switch b.state {
	case CircuitClosed:
		b.failures = 0
	case CircuitHalfOpen:
		b.successes++
		if b.successes >= b.cfg.SuccessThreshold {
			b.state = CircuitClosed
			b.failures = 0
			b.successes = 0
		}
	}
}

func (b *breaker) recordFailure() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.lastFailureTime = time.Now()
	switch b.state {
	case CircuitClosed:
		b.failures++
		if b.failures >= b.cfg.FailureThreshold {
			b.state = CircuitOpen
			b.openedAt = time.Now()
			b.successes = 0
		}
	case CircuitHalfOpen:
		b.state = CircuitOpen
		b.openedAt = time.Now()
		b.successes = 0
	}
}

// Call runs fn if the circuit allows it and records the outcome.
// It returns the circuit-breaker error if the circuit is open, otherwise the
// error from fn (which is also used to trip the breaker).
func (r *Registry) Call(ctx context.Context, provider string, fn func() error) error {
	if err := r.Allow(provider); err != nil {
		return err
	}
	err := fn()
	if err == nil || errors.Is(err, context.Canceled) {
		r.get(provider).recordSuccess()
		return err
	}
	r.get(provider).recordFailure()
	return err
}
