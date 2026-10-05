package wshandler

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"time"

	"github.com/globussoft/callified-backend/internal/audio"
)

const (
	liveUlawFrameBytes      = 160
	liveUlawFrameDuration   = 20 * time.Millisecond
	liveUlawPrebufferFrames = 15 // 300 ms: absorbs ordinary model/network jitter.
	liveUlawMaxBufferFrames = 30 // Never add more than 600 ms latency after a gap.
	liveUlawBufferStep      = 5  // Add 100 ms after a starvation event.
)

type livePlayoutCommand struct {
	epoch uint64
	pcm   []byte
	end   bool
}

// liveAudioPlayout is the sole Tata writer for a Gemini Live response. Gemini
// chunks are intentionally not written directly: their arrival cadence is not
// a telephony clock. This goroutine makes complete 160-byte μ-law frames and
// emits precisely one every 20 ms.
type liveAudioPlayout struct {
	ctx      context.Context
	sess     *CallSession
	commands chan livePlayoutCommand
	done     chan struct{}
	send     func(uint64, []byte)
}

func newLiveAudioPlayout(ctx context.Context, sess *CallSession) *liveAudioPlayout {
	p := &liveAudioPlayout{
		ctx:      ctx,
		sess:     sess,
		commands: make(chan livePlayoutCommand, 512),
		done:     make(chan struct{}),
	}
	p.send = p.sendCarrierFrame
	go p.run()
	return p
}

func (p *liveAudioPlayout) QueuePCM(epoch uint64, pcm8k []byte) bool {
	if len(pcm8k) == 0 {
		return true
	}
	owned := append([]byte(nil), pcm8k...)
	select {
	case p.commands <- livePlayoutCommand{epoch: epoch, pcm: owned}:
		return true
	case <-p.ctx.Done():
		return false
	}
}

// EndTurn is queued after all PCM from a completed Gemini turn. It is the only
// time an incomplete final frame may be zero-padded.
func (p *liveAudioPlayout) EndTurn(epoch uint64) bool {
	select {
	case p.commands <- livePlayoutCommand{epoch: epoch, end: true}:
		return true
	case <-p.ctx.Done():
		return false
	}
}

func (p *liveAudioPlayout) Wait() { <-p.done }

func (p *liveAudioPlayout) run() {
	defer close(p.done)
	framer := audio.NewUlawFramer(liveUlawFrameBytes)
	var (
		epoch          uint64
		haveEpoch      bool
		frames         [][]byte
		playing        bool
		ending         bool
		adaptiveBuffer = liveUlawPrebufferFrames
		startThreshold = adaptiveBuffer
		timer          *time.Timer
		timerC         <-chan time.Time
		nextAt         time.Time
	)

	stopTimer := func() {
		if timer != nil && !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
		timerC = nil
	}
	reset := func(newEpoch uint64, active bool) {
		stopTimer()
		framer.Reset()
		frames = nil
		playing = false
		ending = false
		nextAt = time.Time{}
		epoch = newEpoch
		haveEpoch = active
		startThreshold = adaptiveBuffer
	}
	schedule := func(at time.Time) {
		delay := time.Until(at)
		if delay < 0 {
			delay = 0
		}
		if timer == nil {
			timer = time.NewTimer(delay)
		} else {
			timer.Reset(delay)
		}
		timerC = timer.C
	}
	start := func() {
		if playing || len(frames) == 0 || (len(frames) < startThreshold && !ending) {
			return
		}
		playing = true
		nextAt = time.Now()
		schedule(nextAt)
	}

	for {
		select {
		case <-p.ctx.Done():
			stopTimer()
			return

		case cmd := <-p.commands:
			if cmd.epoch != p.sess.PlaybackEpoch() || p.sess.IsBargeInActive() {
				continue
			}
			if !haveEpoch || epoch != cmd.epoch {
				reset(cmd.epoch, true)
			}
			if cmd.end {
				if final := framer.Flush(); final != nil {
					frames = append(frames, final)
				}
				ending = true
				start()
				continue
			}

			p.sess.AppendTTSChunk(cmd.pcm)
			ulaw := audio.PCMToUlaw(cmd.pcm)
			p.sess.EchoCanceller.FeedTTS(ulaw)
			p.sess.PlaybackTracker.AddBytes(len(ulaw))
			frames = append(frames, framer.Push(ulaw)...)
			start()

		case <-timerC:
			timerC = nil
			if !haveEpoch || epoch != p.sess.PlaybackEpoch() || p.sess.IsBargeInActive() {
				reset(0, false)
				continue
			}
			if len(frames) == 0 {
				if ending {
					reset(epoch, true)
					continue
				}
				// Do not create 20 ms holes inside a word. Pause until a new reserve
				// has accumulated, then restart at normal wall-clock pacing.
				playing = false
				adaptiveBuffer = min(adaptiveBuffer+liveUlawBufferStep, liveUlawMaxBufferFrames)
				startThreshold = adaptiveBuffer
				continue
			}

			frame := frames[0]
			frames = frames[1:]
			p.send(epoch, frame)
			// Keep an absolute clock. If a carrier write is slow, restart the
			// cadence instead of bursting frames to catch up.
			nextAt = nextAt.Add(liveUlawFrameDuration)
			if time.Now().After(nextAt) {
				nextAt = time.Now().Add(liveUlawFrameDuration)
			}
			schedule(nextAt)
		}
	}
}

func (p *liveAudioPlayout) sendCarrierFrame(epoch uint64, frame []byte) {
	if len(frame) != liveUlawFrameBytes || epoch != p.sess.PlaybackEpoch() || p.sess.IsBargeInActive() {
		return
	}
	payload := base64.StdEncoding.EncodeToString(frame)
	media := map[string]any{"payload": payload}
	message := map[string]any{"event": "media", "streamSid": p.sess.StreamSid, "media": media}
	if p.sess.Provider == "tata" {
		seq := p.sess.outboundSeq.Add(1)
		media["chunk"] = seq
		message["stream_sid"] = p.sess.StreamSid
		message["sequenceNumber"] = seq
	}
	wire, err := json.Marshal(message)
	if err == nil && p.sess.SendText(wire) == nil {
		p.sess.MarkAudioSent()
		if p.sess.hasMonitors() {
			p.sess.BroadcastAudio("agent", payload, "ulaw_8k")
		}
	}
}
