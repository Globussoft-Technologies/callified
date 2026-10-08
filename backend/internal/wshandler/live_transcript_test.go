package wshandler

import (
	"encoding/base64"
	"math/rand"
	"testing"

	"go.uber.org/zap"

	"github.com/globussoft/callified-backend/internal/audio"
)

func TestLiveCustomerFinalTranscriptsDoNotWaitForModelTurn(t *testing.T) {
	sess := NewCallSession("test", nil, zap.NewNop())
	if got := commitLiveCustomerTranscript(sess, "  Hello there.  "); got != "Hello there." {
		t.Fatalf("normalized final transcript = %q", got)
	}
	commitLiveCustomerTranscript(sess, "A second answer.")
	commitLiveCustomerTranscript(sess, "  ")
	history := sess.HistorySnapshot()
	if len(history) != 2 {
		t.Fatalf("history has %d turns, want 2 finalized customer segments", len(history))
	}
	if history[0].Role != "user" || history[0].Text != "Hello there." ||
		history[1].Role != "user" || history[1].Text != "A second answer." {
		t.Fatalf("customer transcript segments were merged or changed: %+v", history)
	}
}

func TestSuppressedEchoRemainsInCustomerRecording(t *testing.T) {
	sess := NewCallSession("test", nil, zap.NewNop())
	sess.UseUlaw = true
	carrierAudio := make([]byte, 4000)
	_, _ = rand.New(rand.NewSource(17)).Read(carrierAudio)
	sess.EchoCanceller.FeedTTS(carrierAudio)
	frame := carrierAudio[3200:3360]
	if !sess.EchoCanceller.IsEcho(frame) {
		t.Fatal("test frame must be classified as echo")
	}
	(&Handler{}).handleBinaryFrame(sess, frame)
	if got := sess.echoSuppressedFrames.Load(); got != 1 {
		t.Fatalf("suppressed frames = %d, want 1", got)
	}
	if len(sess.AudioIn) != 0 {
		t.Fatal("echo reached the AI input queue")
	}
	mic, _ := sess.DrainRecordingBuffers()
	if len(mic) != 1 || len(mic[0].Data) != len(audio.UlawToPCM(frame)) {
		t.Fatalf("recording lost customer-channel frame: chunks=%d", len(mic))
	}
}

func TestTataSuppressedEchoRemainsInCustomerRecording(t *testing.T) {
	sess := NewCallSession("test", nil, zap.NewNop())
	sess.UseUlaw = true
	carrierAudio := make([]byte, 4000)
	_, _ = rand.New(rand.NewSource(17)).Read(carrierAudio)
	sess.EchoCanceller.FeedTTS(carrierAudio)
	frame := carrierAudio[3200:3360]
	(&Handler{}).handleMediaEvent(sess, map[string]interface{}{
		"media": map[string]interface{}{"payload": base64.StdEncoding.EncodeToString(frame)},
	})
	if got := sess.echoSuppressedFrames.Load(); got != 1 {
		t.Fatalf("suppressed Tata frames = %d, want 1", got)
	}
	if len(sess.AudioIn) != 0 {
		t.Fatal("Tata echo reached the AI input queue")
	}
	mic, _ := sess.DrainRecordingBuffers()
	if len(mic) != 1 || len(mic[0].Data) != len(audio.UlawToPCM(frame)) {
		t.Fatalf("Tata recording lost customer-channel frame: chunks=%d", len(mic))
	}
}
