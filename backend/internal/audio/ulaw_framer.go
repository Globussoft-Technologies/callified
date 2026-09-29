package audio

// UlawFramer converts arbitrarily sized streaming μ-law chunks into exact
// carrier frames. It retains the incomplete tail between Push calls so chunk
// boundaries from an upstream TTS provider cannot create short packets and
// artificial timing gaps on the phone leg.
type UlawFramer struct {
	frameSize int
	pending   []byte
}

func NewUlawFramer(frameSize int) *UlawFramer {
	if frameSize <= 0 {
		frameSize = 160
	}
	return &UlawFramer{frameSize: frameSize, pending: make([]byte, 0, frameSize)}
}

func (f *UlawFramer) Reset() { f.pending = f.pending[:0] }

// Push returns owned, exact-size frames and keeps any incomplete remainder.
func (f *UlawFramer) Push(data []byte) [][]byte {
	if len(data) == 0 {
		return nil
	}
	f.pending = append(f.pending, data...)
	count := len(f.pending) / f.frameSize
	if count == 0 {
		return nil
	}
	frames := make([][]byte, 0, count)
	for i := 0; i < count; i++ {
		frame := make([]byte, f.frameSize)
		copy(frame, f.pending[i*f.frameSize:(i+1)*f.frameSize])
		frames = append(frames, frame)
	}
	consumed := count * f.frameSize
	copy(f.pending, f.pending[consumed:])
	f.pending = f.pending[:len(f.pending)-consumed]
	return frames
}

// Flush pads the final partial carrier frame with μ-law silence (0xff).
// A nil result means the stream was already frame-aligned.
func (f *UlawFramer) Flush() []byte {
	if len(f.pending) == 0 {
		return nil
	}
	frame := make([]byte, f.frameSize)
	for i := range frame {
		frame[i] = 0xff
	}
	copy(frame, f.pending)
	f.Reset()
	return frame
}
