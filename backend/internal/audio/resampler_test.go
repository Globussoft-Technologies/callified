package audio

import (
	"encoding/binary"
	"math"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDownsampler24To8PreservesChunkContinuity(t *testing.T) {
	input := sinePCM(1000, 24000, 12000, 24000)
	want := NewDownsampler24To8().Process(input)

	d := NewDownsampler24To8()
	var got []byte
	rng := rand.New(rand.NewSource(42))
	for off := 0; off < len(input); {
		n := 1 + rng.Intn(997) // deliberately includes odd byte boundaries
		end := min(off+n, len(input))
		got = append(got, d.Process(input[off:end])...)
		off = end
	}
	assert.Equal(t, want, got)
}

func TestDownsampler24To8FiltersAliases(t *testing.T) {
	const samples = 24000
	pass := NewDownsampler24To8().Process(sinePCM(1000, 24000, 12000, samples))
	stop := NewDownsampler24To8().Process(sinePCM(6000, 24000, 12000, samples))

	// Ignore the short FIR startup transient.
	passRMS := pcmRMS(pass[400:])
	stopRMS := pcmRMS(stop[400:])
	require.Greater(t, passRMS, 7000.0)
	assert.Less(t, stopRMS, passRMS*0.02, "out-of-band energy must not alias into telephone audio")
}

func TestDownsampler24To8OutputDuration(t *testing.T) {
	input := make([]byte, 480*2) // 20 ms at 24 kHz
	got := NewDownsampler24To8().Process(input)
	assert.Len(t, got, 160*2) // 20 ms at 8 kHz
}

func sinePCM(freq, rate float64, amplitude int16, samples int) []byte {
	out := make([]byte, samples*2)
	for i := 0; i < samples; i++ {
		v := int16(math.Round(float64(amplitude) * math.Sin(2*math.Pi*freq*float64(i)/rate)))
		binary.LittleEndian.PutUint16(out[i*2:], uint16(v))
	}
	return out
}

func pcmRMS(pcm []byte) float64 {
	var sum float64
	var n int
	for i := 0; i+1 < len(pcm); i += 2 {
		v := float64(int16(binary.LittleEndian.Uint16(pcm[i:])))
		sum += v * v
		n++
	}
	if n == 0 {
		return 0
	}
	return math.Sqrt(sum / float64(n))
}
