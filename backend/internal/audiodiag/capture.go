// Package audiodiag provides opt-in, private, bounded input-audio diagnostics.
// It never blocks audio forwarding on disk I/O or changes the audio bytes.
package audiodiag

import (
	"encoding/binary"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"
)

const maxStageBytes = 8000 * 2 * 300

var allocationMu sync.Mutex

type entry struct {
	At         time.Time
	Stage      string
	PCM        []byte
	Text       string
	DurationMS float64
}

type Capture struct {
	Dir     string
	start   time.Time
	mu      sync.Mutex
	closed  bool
	queue   chan entry
	done    chan struct{}
	dropped atomic.Uint64
}

// New captures at most five calls per configured directory. Files are PCM16LE,
// mono, 8 kHz. Each capture stops after five minutes or 150,000 events.
func New(root string) *Capture {
	if root == "" {
		return nil
	}
	allocationMu.Lock()
	defer allocationMu.Unlock()
	if os.MkdirAll(root, 0700) != nil {
		return nil
	}
	items, err := os.ReadDir(root)
	if err != nil || len(items) >= 5 {
		return nil
	}
	dir, err := os.MkdirTemp(root, "call-")
	if err != nil {
		return nil
	}
	f, err := os.OpenFile(filepath.Join(dir, "events.jsonl"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return nil
	}
	c := &Capture{Dir: dir, start: time.Now(), queue: make(chan entry, 1024), done: make(chan struct{})}
	go c.run(f)
	return c
}

func (c *Capture) Record(stage string, pcm []byte) {
	c.add(entry{At: time.Now(), Stage: stage, PCM: pcm})
}
func (c *Capture) Event(stage, text string) { c.add(entry{At: time.Now(), Stage: stage, Text: text}) }
func (c *Capture) Timing(stage string, elapsed time.Duration) {
	c.add(entry{At: time.Now(), Stage: stage, DurationMS: float64(elapsed.Microseconds()) / 1000})
}

func (c *Capture) add(e entry) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed || time.Since(c.start) > 300*time.Second {
		return
	}
	// Bound both caller-controlled text and frame size before copying.
	if len(e.PCM) > 64*1024 || len(e.Text) > 8192 {
		c.dropped.Add(1)
		return
	}
	select {
	case c.queue <- entry{At: e.At, Stage: e.Stage, PCM: append([]byte(nil), e.PCM...), Text: e.Text, DurationMS: e.DurationMS}:
	default:
		c.dropped.Add(1)
	}
}

func (c *Capture) Close() {
	if c == nil {
		return
	}
	c.mu.Lock()
	if !c.closed {
		c.closed = true
		close(c.queue)
	}
	c.mu.Unlock()
	<-c.done
}

func (c *Capture) run(events *os.File) {
	defer close(c.done)
	defer events.Close()
	files := map[string]*os.File{}
	offsets := map[string]int{}
	defer func() {
		for _, f := range files {
			_ = f.Close()
		}
	}()
	enc := json.NewEncoder(events)
	_ = enc.Encode(map[string]any{"stage": "format", "sample_rate_hz": 8000, "channels": 1, "encoding": "PCM16LE", "started_utc": c.start.UTC(), "max_seconds": 300, "sent_semantics": "successful local websocket write, not server acknowledgement"})
	count := 0
	for e := range c.queue {
		if count >= 150000 {
			c.dropped.Add(1)
			continue
		}
		count++
		row := map[string]any{"stage": e.Stage, "at_utc": e.At.UTC(), "elapsed_ms": float64(e.At.Sub(c.start).Microseconds()) / 1000}
		if e.Text != "" {
			row["text"] = e.Text
		}
		if e.DurationMS != 0 {
			row["duration_ms"] = e.DurationMS
		}
		if len(e.PCM) > 0 {
			// Only fixed stage names can create files.
			switch e.Stage {
			case "carrier", "echo_removed", "queue_dropped", "queued", "pre_hush", "post_hush", "post_gain", "gemini_sent":
			default:
				c.dropped.Add(1)
				continue
			}
			if offsets[e.Stage]+len(e.PCM) > maxStageBytes {
				c.dropped.Add(1)
				continue
			}
			f := files[e.Stage]
			if f == nil {
				var err error
				f, err = os.OpenFile(filepath.Join(c.Dir, e.Stage+".pcm"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
				if err != nil {
					c.dropped.Add(1)
					continue
				}
				files[e.Stage] = f
			}
			row["offset_bytes"] = offsets[e.Stage]
			n, err := f.Write(e.PCM)
			offsets[e.Stage] += n
			row["bytes"] = n
			if err != nil {
				c.dropped.Add(1)
			}
			var energy float64
			peak := 0
			for i := 0; i+1 < len(e.PCM); i += 2 {
				sample := int(int16(binary.LittleEndian.Uint16(e.PCM[i:])))
				energy += float64(sample) * float64(sample)
				if sample < 0 {
					sample = -sample
				}
				if sample > peak {
					peak = sample
				}
			}
			row["peak"] = peak
			row["rms"] = math.Sqrt(energy / float64(max(1, len(e.PCM)/2)))
		}
		if enc.Encode(row) != nil {
			c.dropped.Add(1)
		}
	}
	_ = enc.Encode(map[string]any{"stage": "capture_summary", "stage_bytes": offsets, "diagnostic_events_dropped": c.dropped.Load(), "events": count})
}
