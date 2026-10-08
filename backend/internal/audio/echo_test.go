package audio

import (
	"encoding/binary"
	"math/rand"
	"testing"
)

func TestEchoLookupUsesTelephonyDecoder(t *testing.T) {
	for value := range 256 {
		decoded := UlawToPCM([]byte{byte(value)})
		want := int16(binary.LittleEndian.Uint16(decoded))
		if got := ulawLinear[value]; got != want {
			t.Fatalf("mu-law byte %#02x decoded to %d, want %d", value, got, want)
		}
	}
	if ulawLinear[0xff] != 0 || ulawLinear[0x7f] != 0 {
		t.Fatalf("mu-law silence decoded incorrectly: ff=%d 7f=%d", ulawLinear[0xff], ulawLinear[0x7f])
	}
}

func TestEchoCancellerDoesNotClassifySilenceAsSpeech(t *testing.T) {
	canceller := NewEchoCanceller()
	silence := make([]byte, 160)
	for i := range silence {
		silence[i] = 0xff
	}
	canceller.FeedTTS(silence)
	if canceller.IsEcho(silence) {
		t.Fatal("silence was suppressed as voiced echo")
	}
}

func TestEchoCancellerStillDetectsMatchingSpeechFrame(t *testing.T) {
	canceller := NewEchoCanceller()
	carrierAudio := make([]byte, echoRingSize)
	_, _ = rand.New(rand.NewSource(17)).Read(carrierAudio)
	canceller.FeedTTS(carrierAudio)
	if !canceller.IsEcho(carrierAudio[3200:3360]) {
		t.Fatal("matching delayed speech frame was not detected as echo")
	}
}
