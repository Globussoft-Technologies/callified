package realtime

import (
	"bytes"
	"testing"
)

func TestLivePCMFormatValidation(t *testing.T) {
	for _, value := range []string{"audio/mpeg", "audio/pcm;rate=bad", "audio/pcm;rate=0", "audio/pcm;channels=2"} {
		if _, err := livePCMRate(value); err == nil {
			t.Fatalf("accepted %q", value)
		}
	}
	rate, err := livePCMRate("audio/pcm;rate=16000")
	if err != nil || rate != 16000 {
		t.Fatalf("%d %v", rate, err)
	}
}

func TestLivePCMNormalizationAcrossChunks(t *testing.T) {
	for _, rate := range []int{8000, 16000, 24000, 44100, 48000} {
		input := make([]byte, rate*2)
		for i := range input {
			input[i] = byte(i)
		}
		var whole, split livePCMNormalizer
		want, _ := whole.process(input, rate)
		var got []byte
		for start := 0; start < len(input); start += 206 {
			end := start + 206
			if end > len(input) {
				end = len(input)
			}
			part, err := split.process(input[start:end], rate)
			if err != nil {
				t.Fatal(err)
			}
			got = append(got, part...)
		}
		if !bytes.Equal(want, got) {
			t.Fatalf("chunk mismatch at %d", rate)
		}
		if count := len(got) / 2; count < 23997 || count > 24000 {
			t.Fatalf("duration changed at %d: %d", rate, count)
		}
	}
}

func TestLiveVoiceSelection(t *testing.T) {
	if ResolveVoice("sarvam-voice", "Kore") != "Kore" {
		t.Fatal("legacy voice leaked")
	}
	if ResolveVoice("en-in-csagent-12", "Kore") != "en-in-csagent-12" {
		t.Fatal("campaign voice lost")
	}
	if ResolveVoice("", "custom-gateway") != "custom-gateway" {
		t.Fatal("gateway voice lost")
	}
}
