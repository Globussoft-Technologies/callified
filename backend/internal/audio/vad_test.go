package audio

import (
	"math"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/assert"
)

// makeSinePCM returns n samples of a sine wave at the given amplitude/frequency.
func makeSinePCM(samples int, amplitude, freqHz float64) []byte {
	out := make([]byte, samples*2)
	for i := 0; i < samples; i++ {
		s := amplitude * math.Sin(2*math.Pi*freqHz*float64(i)/8000.0)
		v := int16(s)
		out[2*i] = byte(v)
		out[2*i+1] = byte(v >> 8)
	}
	return out
}

// makeNoisePCM returns n samples of uniformly-distributed pseudo noise.
func makeNoisePCM(samples int, amplitude float64) []byte {
	out := make([]byte, samples*2)
	for i := 0; i < samples; i++ {
		v := int16(amplitude * (2*rand.Float64() - 1))
		out[2*i] = byte(v)
		out[2*i+1] = byte(v >> 8)
	}
	return out
}

func TestVAD_SilenceAndNoiseNoSpeech(t *testing.T) {
	vad := NewVAD()

	// Digital silence.
	silence := make([]byte, vadFrameBytes*10)
	assert.False(t, vad.ProcessPCM(silence), "silence should not trigger speech")

	// Low-level background noise well below speech thresholds.
	noise := makeNoisePCM(vadFrameSamples*10, 80)
	assert.False(t, vad.ProcessPCM(noise), "quiet noise should not trigger speech")
}

func TestVAD_SpeechOnset(t *testing.T) {
	vad := NewVAD()

	// Prime with silence so the noise floor is known.
	vad.ProcessPCM(make([]byte, vadFrameBytes*5))

	// 200 ms of voiced-ish tone (500 Hz) at a comfortable level.
	speech := makeSinePCM(vadFrameSamples*10, 4000, 500)
	assert.True(t, vad.ProcessPCM(speech), "strong speech-like tone should trigger onset")

	// Continuing speech should NOT re-trigger.
	assert.False(t, vad.ProcessPCM(speech), "continued speech should not re-trigger onset")
}

func TestVAD_NoiseThenSpeech(t *testing.T) {
	vad := NewVAD()

	// 500 ms of moderate background noise to adapt the noise floor.
	noise := makeNoisePCM(vadFrameSamples*25, 400)
	assert.False(t, vad.ProcessPCM(noise), "background noise should not trigger speech")

	// Speech emerges well above the noise floor.
	speech := makeSinePCM(vadFrameSamples*10, 6000, 500)
	assert.True(t, vad.ProcessPCM(speech), "speech above adapted noise floor should trigger")
}

func TestVAD_SilenceResets(t *testing.T) {
	vad := NewVAD()
	vad.ProcessPCM(make([]byte, vadFrameBytes*5))

	speech := makeSinePCM(vadFrameSamples*10, 4000, 500)
	assert.True(t, vad.ProcessPCM(speech))

	// Long silence resets the speech state.
	vad.ProcessPCM(make([]byte, vadFrameBytes*vadSilenceHangover))

	// New utterance should trigger again.
	assert.True(t, vad.ProcessPCM(speech), "new utterance after silence should trigger again")
}

func TestVAD_RemainderBuffersPartialFrame(t *testing.T) {
	vad := NewVAD()
	vad.ProcessPCM(make([]byte, vadFrameBytes*5))

	// Send 1.5 frames worth of audio — no full frame yet.
	partial := makeSinePCM(vadFrameSamples+vadFrameSamples/2, 4000, 500)
	assert.False(t, vad.ProcessPCM(partial), "partial frame should be buffered, not trigger")

	// Finish the second frame.
	rest := makeSinePCM(vadFrameSamples/2+vadFrameSamples*5, 4000, 500)
	assert.True(t, vad.ProcessPCM(rest), "completed frame should now trigger")
}
