package wshandler

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLiveResponseWatchdogRecoversSilentTurn(t *testing.T) {
	recovered := make(chan string, 1)
	w := newLiveResponseWatchdog(15*time.Millisecond, func(text string) bool {
		recovered <- text
		return true
	})
	defer w.Stop()

	w.Arm("  కస్టమర్ ప్రశ్న  ")
	select {
	case text := <-recovered:
		assert.Equal(t, "కస్టమర్ ప్రశ్న", text)
	case <-time.After(time.Second):
		t.Fatal("silent Gemini turn was not recovered")
	}
}

func TestLiveResponseWatchdogCancelsWhenGeminiResponds(t *testing.T) {
	recovered := make(chan string, 1)
	w := newLiveResponseWatchdog(25*time.Millisecond, func(text string) bool {
		recovered <- text
		return true
	})
	defer w.Stop()

	w.Arm("hello")
	w.Cancel()
	select {
	case text := <-recovered:
		require.Failf(t, "unexpected recovery", "watchdog recovered %q after model response", text)
	case <-time.After(75 * time.Millisecond):
	}
}

func TestLiveResponseWatchdogUsesLatestTranscript(t *testing.T) {
	recovered := make(chan string, 1)
	w := newLiveResponseWatchdog(25*time.Millisecond, func(text string) bool {
		recovered <- text
		return true
	})
	defer w.Stop()

	w.Arm("first partial")
	time.Sleep(10 * time.Millisecond)
	w.Arm("first partial completed")
	select {
	case text := <-recovered:
		assert.Equal(t, "first partial completed", text)
	case <-time.After(time.Second):
		t.Fatal("latest transcript was not recovered")
	}
}
