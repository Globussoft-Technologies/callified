package audio

import (
	"encoding/binary"
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

func testTone(frequency float64, samples int) []byte {
	pcm := make([]byte, samples*2)
	for i := 0; i < samples; i++ {
		v := int16(10000 * math.Sin(2*math.Pi*frequency*float64(i)/24000))
		binary.LittleEndian.PutUint16(pcm[i*2:], uint16(v))
	}
	return pcm
}

func TestTelephonyResamplerPreservesChunkBoundaries(t *testing.T) {
	pcm := testTone(1000, 24000)
	var whole, chunked TelephonyResampler
	want := whole.Process(pcm)
	var got []byte
	// Includes splits inside PCM samples and inside a three-sample group.
	for off := 0; off < len(pcm); {
		n := min(137, len(pcm)-off)
		got = append(got, chunked.Process(pcm[off:off+n])...)
		off += n
	}
	require.Len(t, got, 16000)
	require.Equal(t, want, got)
}

func TestTelephonyResamplerRejectsAliasing(t *testing.T) {
	rms := func(pcm []byte) float64 {
		var sum float64
		// Skip the filter's startup transient.
		pcm = pcm[256:]
		for i := 0; i < len(pcm); i += 2 {
			v := float64(int16(binary.LittleEndian.Uint16(pcm[i:])))
			sum += v * v
		}
		return math.Sqrt(sum / float64(len(pcm)/2))
	}
	var speech, highFrequency TelephonyResampler
	pass := rms(speech.Process(testTone(1000, 24000)))
	stop := rms(highFrequency.Process(testTone(5000, 24000)))
	require.Greater(t, pass, 6500.0, "preserve in-band speech")
	require.Less(t, stop, pass*0.02, "prevent high frequencies folding into audible noise")
}
