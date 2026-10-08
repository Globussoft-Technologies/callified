package wshandler

import "strings"

// Gemini Live sends finalized customer transcriptions independently of model
// turnComplete events, with no ordering guarantee. Commit each final segment
// when it arrives instead of buffering it until the model finishes speaking.
func commitLiveCustomerTranscript(sess *CallSession, text string) string {
	text = strings.TrimSpace(text)
	if sess == nil || text == "" {
		return ""
	}
	sess.AppendHistory("user", text)
	sess.BroadcastTranscript("user", text)
	return text
}
