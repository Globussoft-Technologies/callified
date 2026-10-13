package wshandler

import (
	"sync"
	"time"
)

// liveInactivityWatchdog handles a customer who has stopped responding after
// Gemini has completed an agent turn. It intentionally keys off completed
// customer transcripts, not raw carrier audio, so echo and telephone noise do
// not cancel the inactivity flow.
type liveInactivityWatchdog struct {
	mu           sync.Mutex
	initialDelay time.Duration
	retryDelay   time.Duration
	maxPrompts   int
	generation   uint64
	promptCount  int
	active       bool
	timer        *time.Timer
	onPrompt     func(int) bool
	onGoodbye    func()
}

func newLiveInactivityWatchdog(initialDelay, retryDelay time.Duration, maxPrompts int, onPrompt func(int) bool, onGoodbye func()) *liveInactivityWatchdog {
	return &liveInactivityWatchdog{
		initialDelay: initialDelay,
		retryDelay:   retryDelay,
		maxPrompts:   maxPrompts,
		onPrompt:     onPrompt,
		onGoodbye:    onGoodbye,
	}
}

// Arm starts/restarts the initial silence countdown after a completed agent
// response. Once the reminder sequence has begun, only customer speech can
// reset it.
func (w *liveInactivityWatchdog) Arm() {
	w.mu.Lock()
	if w.active {
		w.mu.Unlock()
		return
	}
	w.promptCount = 0
	w.scheduleLocked(w.initialDelay)
	w.mu.Unlock()
}

// CustomerSpoke cancels pending reminders. The next completed agent response
// will arm a fresh 30-second silence window.
func (w *liveInactivityWatchdog) CustomerSpoke() {
	w.mu.Lock()
	w.generation++
	w.promptCount = 0
	w.active = false
	if w.timer != nil {
		w.timer.Stop()
		w.timer = nil
	}
	w.mu.Unlock()
}

func (w *liveInactivityWatchdog) Stop() { w.CustomerSpoke() }

func (w *liveInactivityWatchdog) scheduleLocked(delay time.Duration) {
	w.generation++
	generation := w.generation
	if w.timer != nil {
		w.timer.Stop()
	}
	w.timer = time.AfterFunc(delay, func() { w.fire(generation) })
}

func (w *liveInactivityWatchdog) fire(generation uint64) {
	w.mu.Lock()
	if generation != w.generation {
		w.mu.Unlock()
		return
	}
	w.timer = nil
	w.active = true
	w.promptCount++
	attempt := w.promptCount
	if attempt <= w.maxPrompts {
		w.scheduleLocked(w.retryDelay)
		prompt := w.onPrompt
		w.mu.Unlock()
		if prompt != nil {
			prompt(attempt)
		}
		return
	}
	w.active = false
	w.generation++
	goodbye := w.onGoodbye
	w.mu.Unlock()
	if goodbye != nil {
		goodbye()
	}
}
