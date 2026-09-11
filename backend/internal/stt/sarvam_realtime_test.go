package stt

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSplitSarvamAudioFramesStaysBelowFastModeLimit(t *testing.T) {
	pcm := make([]byte, 11200)
	for i := range pcm {
		pcm[i] = byte(i)
	}

	frames := splitSarvamAudioFrames(pcm)
	require.Len(t, frames, 7)
	for _, frame := range frames {
		assert.LessOrEqual(t, len(frame), sarvamRealtimeAudioFrameBytes)
		assert.Equal(t, 0, len(frame)%2, "linear16 frames must end on a sample boundary")
	}
	assert.True(t, bytes.Equal(pcm, bytes.Join(frames, nil)), "splitting must not lose or reorder audio")
}

func TestSplitSarvamAudioFramesKeepsSmallFrameIntact(t *testing.T) {
	pcm := make([]byte, 640)
	frames := splitSarvamAudioFrames(pcm)

	require.Len(t, frames, 1)
	assert.Equal(t, pcm, frames[0])
	assert.Nil(t, splitSarvamAudioFrames(nil))
}
