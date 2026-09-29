package audio

import "math"

const (
	geminiInputRate   = 24000
	telephonyRate     = 8000
	decimatorFactor   = geminiInputRate / telephonyRate
	decimatorTapCount = 63
)

// Downsampler24To8 converts Gemini Live's 24 kHz PCM16LE stream to telephony
// 8 kHz PCM16LE. It is deliberately stateful: Gemini may split one utterance
// at arbitrary byte/sample boundaries, so both FIR history and decimation
// phase must survive across Process calls.
//
// A Hamming-windowed low-pass filter removes frequencies above the telephone
// passband before decimation. Dropping every third sample without this filter
// aliases high-frequency speech energy into the audible band and produces the
// metallic/crackling sound heard on calls.
type Downsampler24To8 struct {
	taps        [decimatorTapCount]float64
	history     [decimatorTapCount]float64
	historyPos  int // next position to overwrite
	phase       int
	pendingByte byte
	hasPending  bool
}

// NewDownsampler24To8 creates a streaming 24 kHz to 8 kHz converter.
func NewDownsampler24To8() *Downsampler24To8 {
	d := &Downsampler24To8{}
	d.taps = lowPassTaps()
	return d
}

// Reset starts a new discontinuous audio turn without carrying samples from
// the previous (possibly interrupted) Gemini response into the next one.
func (d *Downsampler24To8) Reset() {
	clear(d.history[:])
	d.historyPos = 0
	d.phase = 0
	d.pendingByte = 0
	d.hasPending = false
}

// Process accepts arbitrarily chunked PCM16LE and returns all complete 8 kHz
// output samples available from this chunk. Odd byte boundaries are preserved
// until the next call instead of corrupting sample alignment.
func (d *Downsampler24To8) Process(pcm24k []byte) []byte {
	if len(pcm24k) == 0 {
		return nil
	}

	// At most one output sample is produced for every three input samples.
	out := make([]byte, 0, ((len(pcm24k)+1)/2/decimatorFactor+1)*2)
	consume := func(sample int16) {
		d.history[d.historyPos] = float64(sample)
		d.historyPos = (d.historyPos + 1) % len(d.history)
		d.phase++
		if d.phase != decimatorFactor {
			return
		}
		d.phase = 0

		// historyPos points at the oldest sample; historyPos-1 is newest.
		var filtered float64
		idx := d.historyPos - 1
		if idx < 0 {
			idx = len(d.history) - 1
		}
		for i, tap := range d.taps {
			pos := idx - i
			if pos < 0 {
				pos += len(d.history)
			}
			filtered += d.history[pos] * tap
		}

		v := int64(math.Round(filtered))
		if v > math.MaxInt16 {
			v = math.MaxInt16
		} else if v < math.MinInt16 {
			v = math.MinInt16
		}
		s := uint16(int16(v))
		out = append(out, byte(s), byte(s>>8))
	}

	i := 0
	if d.hasPending {
		consume(int16(uint16(d.pendingByte) | uint16(pcm24k[0])<<8))
		d.hasPending = false
		i = 1
	}
	for ; i+1 < len(pcm24k); i += 2 {
		consume(int16(uint16(pcm24k[i]) | uint16(pcm24k[i+1])<<8))
	}
	if i < len(pcm24k) {
		d.pendingByte = pcm24k[i]
		d.hasPending = true
	}
	return out
}

// Decimate3x is retained for one-shot callers. Streaming call paths must keep
// a Downsampler24To8 instance for the lifetime of an output turn.
func Decimate3x(pcm24k []byte) []byte {
	return NewDownsampler24To8().Process(pcm24k)
}

func lowPassTaps() [decimatorTapCount]float64 {
	var taps [decimatorTapCount]float64
	// Telephone speech is nominally limited to about 3.4 kHz. Keeping the
	// cutoff below the 4 kHz output Nyquist frequency leaves a transition band
	// in which the windowed FIR can attenuate aliases efficiently.
	const cutoffHz = 3400.0
	fc := cutoffHz / geminiInputRate // cycles per input sample (Nyquist = 0.5)
	mid := float64(decimatorTapCount-1) / 2
	var sum float64
	for n := range taps {
		x := float64(n) - mid
		ideal := 2 * fc
		if x != 0 {
			ideal = math.Sin(2*math.Pi*fc*x) / (math.Pi * x)
		}
		window := 0.54 - 0.46*math.Cos(2*math.Pi*float64(n)/float64(decimatorTapCount-1))
		taps[n] = ideal * window
		sum += taps[n]
	}
	// Unity DC gain avoids unintentionally changing speech loudness.
	for i := range taps {
		taps[i] /= sum
	}
	return taps
}
