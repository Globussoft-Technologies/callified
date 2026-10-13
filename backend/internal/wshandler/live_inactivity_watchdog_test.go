package wshandler

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLiveInactivityWatchdogPromptsThreeTimesThenCloses(t *testing.T) {
	prompts := make(chan int, 3)
	goodbye := make(chan struct{}, 1)
	w := newLiveInactivityWatchdog(10*time.Millisecond, 10*time.Millisecond, 3,
		func(attempt int) bool { prompts <- attempt; return true },
		func() { goodbye <- struct{}{} },
	)
	defer w.Stop()
	w.Arm()

	for want := 1; want <= 3; want++ {
		select {
		case got := <-prompts:
			assert.Equal(t, want, got)
		case <-time.After(time.Second):
			t.Fatalf("missing inactivity prompt %d", want)
		}
	}
	select {
	case <-goodbye:
	case <-time.After(time.Second):
		require.Fail(t, "missing inactivity goodbye")
	}
}

func TestLiveInactivityWatchdogCustomerSpeechCancelsSequence(t *testing.T) {
	prompts := make(chan int, 1)
	goodbye := make(chan struct{}, 1)
	w := newLiveInactivityWatchdog(30*time.Millisecond, 10*time.Millisecond, 3,
		func(attempt int) bool { prompts <- attempt; return true },
		func() { goodbye <- struct{}{} },
	)
	defer w.Stop()
	w.Arm()
	w.CustomerSpoke()

	select {
	case attempt := <-prompts:
		t.Fatalf("unexpected prompt after customer speech: %d", attempt)
	case <-goodbye:
		t.Fatal("unexpected goodbye after customer speech")
	case <-time.After(75 * time.Millisecond):
	}
}
