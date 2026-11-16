package realtime

import (
	"encoding/binary"
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

func gainTestTone(amplitude float64, samples int) []byte {
	out := make([]byte, samples*2)
	for i := 0; i < samples; i++ {
		x := int16(math.Round(amplitude * math.Sin(2*math.Pi*220*float64(i)/8000)))
		binary.LittleEndian.PutUint16(out[2*i:], uint16(x))
	}
	return out
}

func gainTestRMS(pcm []byte) float64 {
	var sum float64
	for i := 0; i+1 < len(pcm); i += 2 {
		x := float64(int16(binary.LittleEndian.Uint16(pcm[i:])))
		sum += x * x
	}
	return math.Sqrt(sum / float64(len(pcm)/2))
}

func TestInputGainRaisesBriefQuietSpeechWithoutChangingSourceOrLength(t *testing.T) {
	g := newInputGainController()
	in := gainTestTone(250, 1600) // a short, quiet 200 ms utterance
	original := append([]byte(nil), in...)
	out := g.process(in)
	require.Equal(t, original, in)
	require.Len(t, out, len(in))
	require.Greater(t, gainTestRMS(out), gainTestRMS(in)*3)
	require.LessOrEqual(t, gainTestRMS(out), gainTestRMS(in)*inputGainMaximum+1)
}

func TestInputGainDoesNotBoostSilenceOrLowNoiseAfterSpeech(t *testing.T) {
	g := newInputGainController()
	g.process(gainTestTone(250, 1600))
	silence := make([]byte, 320)
	require.Equal(t, silence, g.process(silence))
	noise := gainTestTone(20, 800)
	require.Equal(t, noise, g.process(noise))
}

func TestInputGainLimitsLoudSpeechAfterQuietSpeech(t *testing.T) {
	g := newInputGainController()
	g.process(gainTestTone(250, 1600))
	out := g.process(gainTestTone(32767, 1600))
	for i := 0; i < len(out); i += 2 {
		v := float64(int16(binary.LittleEndian.Uint16(out[i:])))
		require.LessOrEqual(t, math.Abs(v), inputGainPeak)
	}
	require.Less(t, gainTestRMS(out[len(out)/2:]), gainTestRMS(out[:len(out)/2]))
}

func TestInputGainDisabledAndMalformedInputPassThrough(t *testing.T) {
	var g *inputGainController
	in := []byte{1, 0, 2, 0}
	require.Equal(t, in, g.process(in))
	require.Equal(t, []byte{1}, newInputGainController().process([]byte{1}))
	require.Nil(t, newInputGainController().process(nil))
}

func TestInputGainUsesIndependentStatePerCall(t *testing.T) {
	one, two := newInputGainController(), newInputGainController()
	one.process(gainTestTone(250, 4000))
	require.Equal(t, 1.0, two.gain)
	require.Greater(t, one.gain, two.gain)
}
