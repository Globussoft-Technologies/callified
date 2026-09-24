package wshandler

import (
	"strings"
	"sync"
	"time"
)

const geminiLiveResponseTimeout = 3 * time.Second

// liveResponseWatchdog recovers a Gemini Live turn when meaningful customer
// speech was transcribed but proactive audio produced no voice response. Any
// model output cancels the timer, so normal Live turns are never duplicated.
type liveResponseWatchdog struct {
	mu         sync.Mutex
	delay      time.Duration
	generation uint64
	timer      *time.Timer
	recover    func(string) bool
}

func newLiveResponseWatchdog(delay time.Duration, recover func(string) bool) *liveResponseWatchdog {
	return &liveResponseWatchdog{delay: delay, recover: recover}
}

func (w *liveResponseWatchdog) Arm(customerText string) {
	customerText = strings.TrimSpace(customerText)
	if customerText == "" {
		return
	}

	w.mu.Lock()
	w.generation++
	generation := w.generation
	if w.timer != nil {
		w.timer.Stop()
	}
	w.timer = time.AfterFunc(w.delay, func() {
		w.mu.Lock()
		if generation != w.generation {
			w.mu.Unlock()
			return
		}
		w.timer = nil
		recover := w.recover
		w.mu.Unlock()

		if recover != nil {
			recover(customerText)
		}
	})
	w.mu.Unlock()
}

func (w *liveResponseWatchdog) Cancel() {
	w.mu.Lock()
	w.generation++
	if w.timer != nil {
		w.timer.Stop()
		w.timer = nil
	}
	w.mu.Unlock()
}

func (w *liveResponseWatchdog) Stop() { w.Cancel() }
