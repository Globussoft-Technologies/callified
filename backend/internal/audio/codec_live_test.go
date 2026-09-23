package audio

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGeminiLiveSampleRateConversionSizes(t *testing.T) {
	pcm8k := make([]byte, 160*2) // 20 ms
	pcm16k := Upsample2x(pcm8k)
	assert.Len(t, pcm16k, 320*2)

	pcm24k := make([]byte, 480*2) // 20 ms
	assert.Len(t, Decimate3x(pcm24k), 160*2)
}
