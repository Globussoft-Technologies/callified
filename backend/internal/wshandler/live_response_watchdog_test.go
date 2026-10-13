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

func TestDiscardedLanguageRedirectRecoversMissingReplacement(t *testing.T) {
	recovered := make(chan string, 1)
	w := newLiveResponseWatchdog(15*time.Millisecond, func(text string) bool {
		recovered <- text
		return true
	})
	defer w.Stop()
	w.Arm("previous customer turn")
	w.Cancel() // Redirect and interruption cancel the original deadline.
	w.RecoverDiscardedRedirect("latest language-switch request", false)
	select {
	case text := <-recovered:
		assert.Equal(t, "latest language-switch request", text)
	case <-time.After(time.Second):
		t.Fatal("discarded redirect did not recover its missing replacement")
	}
}

func TestDiscardedLanguageRedirectDoesNotDuplicateReplacementOrClosing(t *testing.T) {
	for _, closing := range []bool{false, true} {
		t.Run(map[bool]string{false: "replacement arrives", true: "call closing"}[closing], func(t *testing.T) {
			recovered := make(chan string, 1)
			w := newLiveResponseWatchdog(20*time.Millisecond, func(text string) bool {
				recovered <- text
				return true
			})
			defer w.Stop()
			w.RecoverDiscardedRedirect("switch language", closing)
			if !closing {
				w.Cancel() // Replacement audio or transcript cancels recovery.
			}
			select {
			case <-recovered:
				t.Fatal("unexpected duplicate or closing-call recovery")
			case <-time.After(60 * time.Millisecond):
			}
		})
	}
}

func TestLiveOutputApprovalDoesNotDependOnTranscriptLanguage(t *testing.T) {
	// No language verdict is needed: short, wrong-language, or absent transcripts
	// must not suppress otherwise valid audio. Redirect holds still apply.
	assert.True(t, liveOutputCanPlay(false, false))
	assert.False(t, liveOutputCanPlay(true, false))
	assert.False(t, liveOutputCanPlay(false, true))
	assert.False(t, liveOutputCanPlay(true, true))
}
