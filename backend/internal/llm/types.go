package llm

import "strings"

// ChatMessage is a single conversation turn stored in session history.
// Role is "user" for the caller or "model" for the AI agent.
// JSON tags are lowercase so persisted transcripts match the Python shape
// ([{"role":"user","text":"..."}]) — the frontend and Python's recording_service
// both expect lowercase keys.
type ChatMessage struct {
	Role string `json:"role"`
	Text string `json:"text"`
}

// SentenceChunk is one complete sentence streamed back from the LLM.
type SentenceChunk struct {
	Text      string
	HasHangup bool // LLM included [HANGUP] → end the call after this sentence
}

// TranscriptRequest carries the input for a single LLM turn.
type TranscriptRequest struct {
	Transcript              string        // current user utterance
	SystemPrompt            string        // system/persona instruction
	History                 []ChatMessage // prior turns (excluding current transcript)
	Language                string        // e.g. "hi", "mr", "en", "ta"
	MaxTokens               int32
	DropIncompleteRemainder bool
	GreetingDone            bool // if true, guardrails will strip repeated greetings
}

// InputText returns the combined input text for token-count estimation.
func (r TranscriptRequest) InputText() string {
	var b strings.Builder
	b.WriteString(r.SystemPrompt)
	b.WriteString("\n")
	for _, m := range r.History {
		b.WriteString(m.Role)
		b.WriteString(": ")
		b.WriteString(m.Text)
		b.WriteString("\n")
	}
	b.WriteString("user: ")
	b.WriteString(r.Transcript)
	return b.String()
}
