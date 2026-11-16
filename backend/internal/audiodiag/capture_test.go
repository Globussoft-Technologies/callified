package audiodiag

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCapturePreservesBytesAndPrivateTimestampedEvents(t *testing.T) {
	c := New(t.TempDir())
	require.NotNil(t, c)
	pcm := []byte{1, 0, 2, 0, 3, 0}
	c.Record("carrier", pcm)
	pcm[0] = 99 // captures must own a copy, never the mutable audio buffer
	c.Event("customer_transcript", "What is your name?")
	c.Close()
	c.Close()
	got, err := os.ReadFile(filepath.Join(c.Dir, "carrier.pcm"))
	require.NoError(t, err)
	require.Equal(t, []byte{1, 0, 2, 0, 3, 0}, got)
	for _, name := range []string{"carrier.pcm", "events.jsonl"} {
		info, err := os.Stat(filepath.Join(c.Dir, name))
		require.NoError(t, err)
		require.Equal(t, os.FileMode(0600), info.Mode().Perm())
	}
	raw, err := os.ReadFile(filepath.Join(c.Dir, "events.jsonl"))
	require.NoError(t, err)
	lines := bytes.Split(bytes.TrimSpace(raw), []byte("\n"))
	require.Len(t, lines, 4)
	var event map[string]any
	require.NoError(t, json.Unmarshal(lines[1], &event))
	require.Equal(t, "carrier", event["stage"])
	require.NotEmpty(t, event["at_utc"])
	require.Equal(t, float64(0), event["offset_bytes"])
	require.Equal(t, float64(6), event["bytes"])
}

func TestCaptureLimitsAndDisabledMode(t *testing.T) {
	var disabled *Capture
	disabled.Record("carrier", []byte{1, 2})
	disabled.Event("test", "")
	disabled.Close()
	root := t.TempDir()
	for i := 0; i < 5; i++ {
		c := New(root)
		require.NotNil(t, c)
		c.Close()
	}
	require.Nil(t, New(root))
	c := &Capture{start: time.Now().Add(-301 * time.Second), queue: make(chan entry, 1)}
	c.Record("carrier", []byte{1, 0})
	require.Empty(t, c.queue)
}

func TestCaptureConcurrentCloseCannotBlockAudioProducers(t *testing.T) {
	c := New(t.TempDir())
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				c.Record("pre_hush", []byte{1, 0})
			}
		}()
	}
	c.Close()
	wg.Wait()
}

func TestFullDiagnosticQueueDropsOnlyDiagnosticCopy(t *testing.T) {
	c := &Capture{start: time.Now(), queue: make(chan entry, 1)}
	c.Record("carrier", []byte{1, 0})
	c.Record("carrier", []byte{2, 0})
	require.Equal(t, uint64(1), c.dropped.Load())
}
