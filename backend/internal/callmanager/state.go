package callmanager

import (
	"fmt"
	"sync"
	"time"
)

// State represents the lifecycle of a single call.
type State string

const (
	StatePending    State = "pending"
	StateDialing    State = "dialing"
	StateConnected  State = "connected"
	StateSpeaking   State = "speaking"
	StateListening  State = "listening"
	StateCompleted  State = "completed"
	StateFailed     State = "failed"
	StateNoAnswer   State = "no_answer"
	StateBusy       State = "busy"
	StateVoicemail  State = "voicemail"
	StateHangup     State = "hangup"
)

// IsTerminal returns true if the call has reached a final state.
func (s State) IsTerminal() bool {
	switch s {
	case StateCompleted, StateFailed, StateNoAnswer, StateBusy, StateVoicemail, StateHangup:
		return true
	default:
		return false
	}
}

// validTransitions defines allowed state machine transitions.
var validTransitions = map[State]map[State]bool{
	StatePending:   {StateDialing: true},
	StateDialing:   {StateConnected: true, StateFailed: true, StateNoAnswer: true, StateBusy: true},
	StateConnected: {StateSpeaking: true, StateListening: true, StateCompleted: true, StateFailed: true, StateVoicemail: true, StateHangup: true},
	StateSpeaking:  {StateListening: true, StateCompleted: true, StateFailed: true, StateHangup: true},
	StateListening: {StateSpeaking: true, StateCompleted: true, StateFailed: true, StateHangup: true},
	StateCompleted: {},
	StateFailed:    {},
	StateNoAnswer:  {},
	StateBusy:      {},
	StateVoicemail: {},
	StateHangup:    {},
}

// Manager tracks the state of one call with thread-safe transitions and timestamps.
type Manager struct {
	mu        sync.RWMutex
	state     State
	startedAt time.Time
	transitions []Transition
}

// Transition records a single state change.
type Transition struct {
	From      State
	To        State
	Timestamp time.Time
	Reason    string
}

// NewManager creates a call state manager starting in StatePending.
func NewManager() *Manager {
	return &Manager{
		state:     StatePending,
		startedAt: time.Now(),
		transitions: []Transition{},
	}
}

// Transition attempts to move the call to newState. It returns an error if the
// transition is not allowed.
func (m *Manager) Transition(newState State, reason string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.state.IsTerminal() {
		return fmt.Errorf("cannot transition from terminal state %s", m.state)
	}
	if !validTransitions[m.state][newState] {
		return fmt.Errorf("invalid transition from %s to %s", m.state, newState)
	}
	m.transitions = append(m.transitions, Transition{
		From:      m.state,
		To:        newState,
		Timestamp: time.Now(),
		Reason:    reason,
	})
	m.state = newState
	return nil
}

// State returns the current state.
func (m *Manager) State() State {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.state
}

// Transitions returns a copy of the transition history.
func (m *Manager) Transitions() []Transition {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]Transition, len(m.transitions))
	copy(out, m.transitions)
	return out
}

// StartedAt returns the time the manager was created.
func (m *Manager) StartedAt() time.Time {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.startedAt
}

// Duration returns the elapsed time since the manager was created.
func (m *Manager) Duration() time.Duration {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return time.Since(m.startedAt)
}
