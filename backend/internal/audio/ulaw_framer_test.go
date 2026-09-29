package audio

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUlawFramerCarriesChunkRemainders(t *testing.T) {
	f := NewUlawFramer(160)
	input := make([]byte, 355)
	for i := range input {
		input[i] = byte(i)
	}

	var frames [][]byte
	for _, chunk := range [][]byte{input[:37], input[37:199], input[199:321], input[321:]} {
		frames = append(frames, f.Push(chunk)...)
	}
	require.Len(t, frames, 2)
	assert.Len(t, frames[0], 160)
	assert.Len(t, frames[1], 160)
	assert.Equal(t, input[:320], bytes.Join(frames, nil))

	last := f.Flush()
	require.Len(t, last, 160)
	assert.Equal(t, input[320:], last[:35])
	assert.Equal(t, bytes.Repeat([]byte{0xff}, 125), last[35:])
	assert.Nil(t, f.Flush())
}

func TestUlawFramerResetDropsInterruptedTail(t *testing.T) {
	f := NewUlawFramer(160)
	assert.Empty(t, f.Push(bytes.Repeat([]byte{1}, 80)))
	f.Reset()
	frames := f.Push(bytes.Repeat([]byte{2}, 160))
	require.Len(t, frames, 1)
	assert.Equal(t, bytes.Repeat([]byte{2}, 160), frames[0])
}
