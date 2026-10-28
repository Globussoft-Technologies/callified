package audio

import (
	"encoding/binary"
	"math"
)

const telephonyFilterTaps = 63

var telephonyFilter = func() [telephonyFilterTaps]float64 {
	// Windowed sinc low-pass at 3.4 kHz before decimating 24 kHz to 8 kHz.
	// Filtering prevents frequencies above the telephone band folding into it.
	var taps [telephonyFilterTaps]float64
	const cutoff = 3400.0 / 24000
	var sum float64
	for i := range taps {
		x := float64(i - (telephonyFilterTaps-1)/2)
		v := 2 * cutoff
		if x != 0 {
			v = math.Sin(2*math.Pi*cutoff*x) / (math.Pi * x)
		}
		v *= 0.54 - 0.46*math.Cos(2*math.Pi*float64(i)/(telephonyFilterTaps-1))
		taps[i] = v
		sum += v
	}
	for i := range taps {
		taps[i] /= sum
	}
	return taps
}()

// TelephonyResampler converts a stream of 24 kHz PCM16LE to 8 kHz. Its zero
// value is ready to use. Keep one per output turn: filter history, decimation
// phase, and even a split PCM sample survive arbitrary network chunk boundaries.
// It must be called serially, like the realtime client's audio callbacks.
type TelephonyResampler struct {
	history [telephonyFilterTaps]float64
	pos     int
	phase   int
	low     byte
	hasLow  bool
}

func (r *TelephonyResampler) Process(pcm []byte) []byte {
	out := make([]byte, 0, (len(pcm)/6+1)*2)
	for _, b := range pcm {
		if !r.hasLow {
			r.low = b
			r.hasLow = true
			continue
		}
		sample := int16(uint16(r.low) | uint16(b)<<8)
		r.hasLow = false
		r.history[r.pos] = float64(sample)
		r.pos = (r.pos + 1) % telephonyFilterTaps
		r.phase++
		if r.phase != 3 {
			continue
		}
		r.phase = 0
		var filtered float64
		for i, tap := range telephonyFilter {
			index := (r.pos - 1 - i + telephonyFilterTaps) % telephonyFilterTaps
			filtered += tap * r.history[index]
		}
		v := int16(math.Round(math.Max(-32768, math.Min(32767, filtered))))
		out = binary.LittleEndian.AppendUint16(out, uint16(v))
	}
	return out
}
