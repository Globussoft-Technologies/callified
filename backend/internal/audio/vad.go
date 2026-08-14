package audio

import (
	"math"
)

const (
	// vadFrameSamples is the number of 16-bit PCM samples in one VAD frame.
	// At 8 kHz, 20 ms = 160 samples.
	vadFrameSamples = 160
	vadFrameBytes   = vadFrameSamples * 2 // 16-bit LE

	// Conservative defaults tuned for telephony at 8 kHz mono.
	// A frame is classified as speech only when ALL of these hold:
	//   - RMS > vadMinEnergy (absolute floor, excludes digital silence)
	//   - RMS / noiseFloor > vadMinSNR (adaptive, excludes steady background noise)
	//   - zeroCrossingRate in [vadMinZCR, vadMaxZCR] (speech vs tonal/hiss noise)
	// After that, vadSpeechHangover consecutive speech frames are required before
	// declaring speech onset, which rejects impulse clicks.
	vadMinEnergy       = 100.0  // int16 RMS
	vadMinSNR          = 12.0  // linear ratio (~21 dB above estimated noise)
	vadMinZCR          = 5      // per 20 ms frame
	vadMaxZCR          = 60     // per 20 ms frame
	vadSpeechHangover  = 3      // 60 ms of continuous speech
	vadSilenceHangover = 25     // 500 ms before clearing speech state
	vadNoiseAlpha      = 0.95   // EMA weight for noise-floor updates
	vadMinNoiseFloor   = 50.0   // prevents division by zero / underwater floors
)

// VAD is a lightweight, pure-Go voice-activity detector for 8 kHz 16-bit PCM.
//
// It is intentionally simple (no ML, no CGO) but far more noise-robust than a
// raw energy threshold: it combines an adaptive noise floor, SNR gating,
// zero-crossing-rate filtering, and a short hangover.
//
// VAD is NOT safe for concurrent use; a CallSession uses it from a single
// WebSocket read goroutine.
type VAD struct {
	noiseFloor   float64
	speechCount  int
	silenceCount int
	inSpeech     bool
	remainder    []byte
}

// NewVAD creates a VAD with the default noise-floor prior.
func NewVAD() *VAD {
	return &VAD{noiseFloor: vadMinNoiseFloor}
}

// Reset clears internal state. Call when the call audio path reconnects.
func (v *VAD) Reset() {
	v.noiseFloor = vadMinNoiseFloor
	v.speechCount = 0
	v.silenceCount = 0
	v.inSpeech = false
	v.remainder = nil
}

// ProcessPCM consumes a chunk of 8 kHz 16-bit LE PCM and returns true once when
// speech onset is detected. Subsequent frames in the same utterance do not
// return true again; the detector resets after enough silence.
func (v *VAD) ProcessPCM(pcm []byte) bool {
	v.remainder = append(v.remainder, pcm...)

	onset := false
	for len(v.remainder) >= vadFrameBytes {
		frame := v.remainder[:vadFrameBytes]
		if v.processFrame(frame) && !onset {
			onset = true
		}
		v.remainder = v.remainder[vadFrameBytes:]
	}

	return onset
}

// processFrame evaluates a single 20 ms frame. Returns true on speech onset.
func (v *VAD) processFrame(frame []byte) bool {
	rms := frameRMS(frame)
	zcr := frameZCR(frame)

	// Absolute silence: never speech, but update noise floor if we are quiet.
	isSilence := rms < vadMinEnergy

	if isSilence {
		v.silenceCount++
		v.speechCount = 0
		// Adapt noise floor during quiet periods only, and not while we believe
		// we are inside speech (which would pull the floor up).
		if !v.inSpeech {
			v.noiseFloor = vadNoiseAlpha*v.noiseFloor + (1-vadNoiseAlpha)*rms
			if v.noiseFloor < vadMinNoiseFloor {
				v.noiseFloor = vadMinNoiseFloor
			}
		}
	} else {
		v.silenceCount = 0
	}

	if v.silenceCount >= vadSilenceHangover {
		v.inSpeech = false
	}

	// Speech decision.
	snr := rms / v.noiseFloor
	isSpeechFrame := !isSilence && snr > vadMinSNR && zcr >= vadMinZCR && zcr <= vadMaxZCR

	if isSpeechFrame {
		v.speechCount++
	} else {
		v.speechCount = 0
	}

	if v.speechCount >= vadSpeechHangover && !v.inSpeech {
		v.inSpeech = true
		return true
	}
	return false
}

// NoiseFloor returns the current estimated noise RMS. Exposed for tests/metrics.
func (v *VAD) NoiseFloor() float64 { return v.noiseFloor }

// frameRMS returns the root-mean-square of int16 samples in a frame.
func frameRMS(frame []byte) float64 {
	n := len(frame) / 2
	if n == 0 {
		return 0
	}
	var sum float64
	for i := 0; i < n; i++ {
		s := float64(int16(frame[2*i]) | int16(frame[2*i+1])<<8)
		sum += s * s
	}
	return math.Sqrt(sum / float64(n))
}

// frameZCR returns the number of zero-crossings in a frame.
func frameZCR(frame []byte) int {
	n := len(frame) / 2
	if n < 2 {
		return 0
	}
	prev := int16(frame[0]) | int16(frame[1])<<8
	cross := 0
	for i := 1; i < n; i++ {
		s := int16(frame[2*i]) | int16(frame[2*i+1])<<8
		if (prev >= 0 && s < 0) || (prev < 0 && s >= 0) {
			cross++
		}
		prev = s
	}
	return cross
}
