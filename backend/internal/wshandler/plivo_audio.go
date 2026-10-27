package wshandler

import (
	"encoding/base64"
	"encoding/json"
	"sync"
	"time"

	"github.com/globussoft/callified-backend/internal/audio"
	"go.uber.org/zap"
)

// Plivo plays raw G.711 audio from playAudio events, rather than media events.
func plivoAudioFrame(ulaw []byte) map[string]interface{} {
	return map[string]interface{}{
		"event": "playAudio",
		"media": map[string]interface{}{
			"contentType": "audio/x-mulaw",
			"sampleRate":  8000,
			"payload":     base64.StdEncoding.EncodeToString(ulaw),
		},
	}
}

type plivoPlayback struct {
	mu       sync.Mutex
	epoch    uint64
	pcm      []byte
	nextSend time.Time
}

// sendPlivoPCM joins provider chunks before packetizing. Short chunks must not
// each introduce a full 20 ms pause. Only the final packet of a turn is padded.
// Deadlines include write time so small per-frame delays do not accumulate.
func sendPlivoPCM(sess *CallSession, pcm []byte, epoch uint64, flush bool) {
	p := &sess.plivoOutput
	p.mu.Lock()
	defer p.mu.Unlock()
	if epoch != sess.PlaybackEpoch() || sess.IsBargeInActive() {
		return
	}
	if p.epoch != epoch {
		p.pcm = nil
		p.nextSend = time.Time{}
		p.epoch = epoch
	}
	p.pcm = append(p.pcm, pcm...)
	const pcmFrameBytes = 320 // 160 PCM16 samples at 8 kHz
	if flush && len(p.pcm)%pcmFrameBytes != 0 {
		p.pcm = append(p.pcm, make([]byte, pcmFrameBytes-len(p.pcm)%pcmFrameBytes)...)
	}
	for len(p.pcm) >= pcmFrameBytes {
		if delay := time.Until(p.nextSend); delay > 0 {
			time.Sleep(delay)
		}
		if epoch != sess.PlaybackEpoch() || sess.IsBargeInActive() {
			p.pcm = nil
			p.nextSend = time.Time{}
			return
		}
		now := time.Now()
		if p.nextSend.IsZero() || now.Sub(p.nextSend) >= 20*time.Millisecond {
			p.nextSend = now
		}
		framePCM := p.pcm[:pcmFrameBytes]
		ulaw := audio.PCMToUlaw(framePCM)
		frame, _ := json.Marshal(plivoAudioFrame(ulaw))
		if err := sess.SendText(frame); err != nil {
			sess.Log.Warn("Plivo audio write failed", zap.Error(err))
			p.pcm = nil
			return
		}
		sess.AppendTTSChunk(framePCM)
		sess.EchoCanceller.FeedTTS(ulaw)
		sess.PlaybackTracker.AddBytes(len(ulaw))
		sess.MarkAudioSent()
		sess.MarkFirstLiveAudioSent()
		if sess.hasMonitors() {
			sess.BroadcastAudio("agent", base64.StdEncoding.EncodeToString(ulaw), "ulaw_8k")
		}
		p.pcm = p.pcm[pcmFrameBytes:]
		p.nextSend = p.nextSend.Add(20 * time.Millisecond)
	}
	if len(p.pcm) == 0 {
		p.pcm = nil
	}
}
