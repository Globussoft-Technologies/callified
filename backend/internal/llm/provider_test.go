package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/globussoft/callified-backend/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestSanitizeRepeatIntentKey(t *testing.T) {
	assert.Equal(t, "ask_price", sanitizeRepeatIntentKey("`Ask Price.`"))
	assert.Equal(t, "ask_biometrics_meaning", sanitizeRepeatIntentKey("ask biometrics meaning"))
	assert.Empty(t, sanitizeRepeatIntentKey("none"))
	assert.Empty(t, sanitizeRepeatIntentKey("Greeting."))
}

func TestBracketedMetaFilterStripsBeforeSentenceSplitting(t *testing.T) {
	filter := newBracketedMetaFilter()

	assert.Equal(t, "", filter.Write("[Customer interrupted. "))
	assert.Equal(t, "", filter.Write("The agent needs to re-ask the current question.]"))
	assert.Equal(t, " Okay sri.", filter.Write(" Okay sri."))
}

func TestBracketedMetaFilterPreservesHangupSignal(t *testing.T) {
	filter := newBracketedMetaFilter()

	assert.Equal(t, "Thank you. ", filter.Write("Thank you. "))
	assert.Equal(t, "[HANGUP]", filter.Write("[HANGUP]"))
}

func TestParseChunkStripsUnbracketedControlNarration(t *testing.T) {
	text, hangup := parseChunk(`: The customer's response "How many" is unclear. It does not directly answer my previous question. I need to re-ask it. I will keep it simple. శ్రీ గారు, మీరు అటెండెన్స్ కోసం చూస్తున్నారా?`)

	assert.False(t, hangup)
	assert.Equal(t, "శ్రీ గారు, మీరు అటెండెన్స్ కోసం చూస్తున్నారా?", text)
}

func TestParseChunkStripsAgentNeedsNarration(t *testing.T) {
	text, hangup := parseChunk(`The agent needs to re-ask the current question. ] Okay sri. Are you looking for attendance or access management?`)

	assert.False(t, hangup)
	assert.Equal(t, "Okay sri. Are you looking for attendance or access management?", text)
}

func TestSpokenEnvelopeDropsScreenshotReasoning(t *testing.T) {
	filter := newSpokenEnvelopeFilter()
	chunks := []string{
		`The user said "Okay, fine". This is an affirmative signal according to the FORWARD SIGNAL rule. `,
		`I should proceed to the next step. I will ask when they are free. <SA`,
		`Y>సరే శ్రీ గారు, డెమో కోసం ఈరోజు లేదా రేపు ఎప్పుడు free గా ఉంటారు?</S`,
		`AY> This text must also be dropped.`,
	}
	var out strings.Builder
	for _, chunk := range chunks {
		out.WriteString(filter.Write(chunk))
	}

	assert.True(t, filter.SawEnvelope())
	assert.Equal(t, "సరే శ్రీ గారు, డెమో కోసం ఈరోజు లేదా రేపు ఎప్పుడు free గా ఉంటారు?", out.String())
	assert.NotContains(t, out.String(), "FORWARD SIGNAL")
}

func TestSpokenEnvelopeWorksForEverySupportedLanguage(t *testing.T) {
	replies := map[string]string{
		"en": "When are you free?",
		"hi": "आप कब free हैं?",
		"mr": "तुम्ही कधी free आहात?",
		"bn": "আপনি কখন free আছেন?",
		"gu": "તમે ક્યારે free છો?",
		"pa": "ਤੁਸੀਂ ਕਦੋਂ free ਹੋ?",
		"ta": "நீங்கள் எப்போது free?",
		"te": "మీరు ఎప్పుడు free గా ఉంటారు?",
		"kn": "ನೀವು ಯಾವಾಗ free ಇದ್ದೀರಿ?",
		"ml": "നിങ്ങൾ എപ്പോഴാണ് free?",
	}
	for language, reply := range replies {
		t.Run(language, func(t *testing.T) {
			filter := newSpokenEnvelopeFilter()
			got := filter.Write("internal analysis <SAY>" + reply + "</SAY> internal notes")
			assert.Equal(t, reply, got)
			assert.True(t, filter.SawEnvelope())
		})
	}
}

func TestSpokenEnvelopePreservesHangup(t *testing.T) {
	filter := newSpokenEnvelopeFilter()
	assert.Equal(t, "Thank you. [HANGUP]", filter.Write("<SAY>Thank you. [HANGUP]</SAY>"))
	assert.True(t, filter.ClosedEnvelope())
}

func TestSpokenEnvelopeRejectsUntaggedOutput(t *testing.T) {
	filter := newSpokenEnvelopeFilter()
	assert.Empty(t, filter.Write("This means the question is answered. I should continue."))
	assert.False(t, filter.SawEnvelope())
}

func TestVoiceOutputProtocolIsAppendedLast(t *testing.T) {
	assert.Contains(t, voiceOutputProtocol, "<SAY>")
	assert.Contains(t, voiceOutputProtocol, "Do not output reasoning")
}

func TestProcessTranscriptUsesRetryForEmptyEnvelope(t *testing.T) {
	provider, closeServer := testGeminiProvider(t, "<SAY></SAY>")
	defer closeServer()

	var chunks []SentenceChunk
	err := provider.ProcessTranscript(context.Background(), TranscriptRequest{
		Transcript: "hello", Language: "te", MaxTokens: 100,
	}, func(chunk SentenceChunk) { chunks = append(chunks, chunk) })

	require.ErrorIs(t, err, ErrEmptySpokenResponse)
	require.Len(t, chunks, 1)
	assert.Equal(t, safeVoiceRetry("te"), chunks[0].Text)
}

func TestProcessTranscriptFlushesClosedUnpunctuatedEnvelope(t *testing.T) {
	provider, closeServer := testGeminiProvider(t, "<SAY>మీరు ఎప్పుడు free గా ఉంటారు</SAY>")
	defer closeServer()

	var chunks []SentenceChunk
	err := provider.ProcessTranscript(context.Background(), TranscriptRequest{
		Transcript: "hello", Language: "te", MaxTokens: 100, DropIncompleteRemainder: true,
	}, func(chunk SentenceChunk) { chunks = append(chunks, chunk) })

	require.NoError(t, err)
	require.Len(t, chunks, 1)
	assert.Equal(t, "మీరు ఎప్పుడు free గా ఉంటారు", chunks[0].Text)
}

func TestProcessTranscriptRetriesMissingEnvelopeBeforeSpeakingFallback(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Header().Set("Content-Type", "text/event-stream")
		output := "I should ask the customer to confirm again."
		if requests == 2 {
			output = "<SAY>Great! Let me tell you more.</SAY>"
		}
		_, _ = fmt.Fprintf(w, "data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":%q}]}}]}\n\n", output)
	}))
	defer server.Close()

	provider := NewProvider(&config.Config{
		GeminiAPIKey:  "test-key",
		GeminiModel:   "gemini-2.5-flash",
		GeminiBaseURL: server.URL,
		LLMProvider:   "gemini",
	}, zap.NewNop())
	var chunks []SentenceChunk
	err := provider.ProcessTranscript(context.Background(), TranscriptRequest{
		Transcript: "Yes, I did.", Language: "en", MaxTokens: 100,
	}, func(chunk SentenceChunk) { chunks = append(chunks, chunk) })

	require.NoError(t, err)
	assert.Equal(t, 2, requests)
	require.Len(t, chunks, 2)
	assert.Equal(t, "Great! Let me tell you more.", chunks[0].Text+" "+chunks[1].Text)
	for _, chunk := range chunks {
		assert.NotEqual(t, safeVoiceRetry("en"), chunk.Text)
	}
}

func TestProcessTranscriptDeliversSafeUntaggedReplyWithoutRetry(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, "data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"Great! What would you like to know?\"}]}}]}\n\n")
	}))
	defer server.Close()

	provider := NewProvider(&config.Config{
		GeminiAPIKey: "test-key", GeminiModel: "gemini-2.5-flash",
		GeminiBaseURL: server.URL, LLMProvider: "gemini",
	}, zap.NewNop())
	var chunks []SentenceChunk
	err := provider.ProcessTranscript(context.Background(), TranscriptRequest{
		Transcript: "Yes", Language: "en", MaxTokens: 100,
	}, func(chunk SentenceChunk) { chunks = append(chunks, chunk) })

	require.NoError(t, err)
	assert.Equal(t, 1, requests)
	require.Len(t, chunks, 2)
	assert.Equal(t, "Great! What would you like to know?", chunks[0].Text+" "+chunks[1].Text)
}

func TestSanitizeUntaggedVoiceResponseRemovesScreenshotNarration(t *testing.T) {
	response := `The user said "Okay, fine" after I re-asked the question. This is an affirmative signal according to the FORWARD SIGNAL rule. I should proceed to the next step. I will ask when they are free. సరే శ్రీ గారు, డెమో కోసం ఎప్పుడు free గా ఉంటారు?`

	assert.Equal(t, "సరే శ్రీ గారు, డెమో కోసం ఎప్పుడు free గా ఉంటారు?", sanitizeUntaggedVoiceResponse(response))
}

func TestSanitizeUntaggedVoiceResponseRejectsAnalysisOnly(t *testing.T) {
	response := `The user said yes. I should proceed to the next step in the call flow.`

	assert.Empty(t, sanitizeUntaggedVoiceResponse(response))
}

func TestOrphanVoiceEnvelopeTagsNeverReachOutput(t *testing.T) {
	input := `Perfect. I'll send the invite. Thank you. </SAY>`

	assert.Equal(t, "Perfect. I'll send the invite. Thank you.", sanitizeUntaggedVoiceResponse(input))
	text, hangup := parseChunk(`Thank you, sri. < SAY >`)
	assert.False(t, hangup)
	assert.Equal(t, "Thank you, sri.", text)
}

func TestProcessTranscriptDeliversStructuredCompleteCall(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		assert.NotEmpty(t, body["tools"])
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, `data: {"candidates":[{"content":{"parts":[{"functionCall":{"name":"complete_call","args":{"spoken_text":"Your demo is confirmed. Thank you.","outcome":"appointment_booked"}}}]}}]}`+"\n\n")
	}))
	defer server.Close()

	provider := NewProvider(&config.Config{
		GeminiAPIKey: "test-key", GeminiModel: "gemini-2.5-flash",
		GeminiBaseURL: server.URL, LLMProvider: "gemini",
	}, zap.NewNop())
	var chunks []SentenceChunk
	err := provider.ProcessTranscript(context.Background(), TranscriptRequest{
		Transcript: "Yes, confirm it.", Language: "en", MaxTokens: 100,
	}, func(chunk SentenceChunk) { chunks = append(chunks, chunk) })

	require.NoError(t, err)
	require.Len(t, chunks, 1)
	assert.Equal(t, "Your demo is confirmed. Thank you.", chunks[0].Text)
	assert.True(t, chunks[0].HasHangup)
	assert.Equal(t, "appointment_booked", chunks[0].HangupOutcome)
}

func TestProcessTranscriptDoesNotDuplicateTextBeforeStructuredAction(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, `data: {"candidates":[{"content":{"parts":[{"text":"<SAY>Your demo is confirmed. Thank you.</SAY>"},{"functionCall":{"name":"complete_call","args":{"spoken_text":"Your demo is confirmed. Thank you.","outcome":"appointment_booked"}}}]}}]}`+"\n\n")
	}))
	defer server.Close()

	provider := NewProvider(&config.Config{
		GeminiAPIKey: "test-key", GeminiModel: "gemini-2.5-flash",
		GeminiBaseURL: server.URL, LLMProvider: "gemini",
	}, zap.NewNop())
	var chunks []SentenceChunk
	err := provider.ProcessTranscript(context.Background(), TranscriptRequest{
		Transcript: "Yes, tomorrow at 4 PM.", Language: "en", MaxTokens: 100,
	}, func(chunk SentenceChunk) { chunks = append(chunks, chunk) })

	require.NoError(t, err)
	var spoken strings.Builder
	hangups := 0
	for _, chunk := range chunks {
		spoken.WriteString(chunk.Text)
		if chunk.HasHangup {
			hangups++
			assert.Equal(t, "appointment_booked", chunk.HangupOutcome)
		}
	}
	assert.Equal(t, "Your demo is confirmed.Thank you.", spoken.String())
	assert.Equal(t, 1, hangups)
}

func TestCollapseExactRepeatedVoiceText(t *testing.T) {
	confirmation := "Perfect. Your demo is confirmed for tomorrow at four PM. Thank you for your time."
	assert.Equal(t, confirmation, collapseExactRepeatedVoiceText(confirmation+" "+confirmation))
	assert.Equal(t, "Yes, yes.", collapseExactRepeatedVoiceText("Yes, yes."))
}

func TestProcessTranscriptTimeoutReleasesTheNextTurn(t *testing.T) {
	var requests atomic.Int32
	releaseFirst := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if requests.Add(1) == 1 {
			<-releaseFirst
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, "data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"<SAY>I can hear you now.</SAY>\"}]}}]}\n\n")
	}))
	defer func() {
		close(releaseFirst)
		server.Close()
	}()

	provider := NewProvider(&config.Config{
		GeminiAPIKey: "test-key", GeminiModel: "gemini-2.5-flash",
		GeminiBaseURL: server.URL, LLMProvider: "gemini",
	}, zap.NewNop())
	provider.voiceAttemptTimeout = 50 * time.Millisecond

	var first []SentenceChunk
	err := provider.ProcessTranscript(context.Background(), TranscriptRequest{
		Transcript: "Tell me about the features", Language: "en", MaxTokens: 100,
	}, func(chunk SentenceChunk) { first = append(first, chunk) })
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Len(t, first, 1)
	assert.Equal(t, safeVoiceRetry("en"), first[0].Text)

	var second []SentenceChunk
	err = provider.ProcessTranscript(context.Background(), TranscriptRequest{
		Transcript: "Hello", Language: "en", MaxTokens: 100,
	}, func(chunk SentenceChunk) { second = append(second, chunk) })
	require.NoError(t, err)
	require.Len(t, second, 1)
	assert.Equal(t, "I can hear you now.", second[0].Text)
}

func TestProcessTranscriptFallsBackToGroqBeforeSpeaking(t *testing.T) {
	releaseGemini := make(chan struct{})
	gemini := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		<-releaseGemini
	}))
	defer func() {
		close(releaseGemini)
		gemini.Close()
	}()

	groq := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"<SAY>Here are the CRM features.</SAY>\"},\"finish_reason\":null}]}\n\n")
	}))
	defer groq.Close()

	provider := NewProvider(&config.Config{
		GeminiAPIKey: "gemini-key", GeminiModel: "gemini-2.5-flash", GeminiBaseURL: gemini.URL,
		GroqAPIKey: "groq-key", GroqModel: "test-model", LLMProvider: "gemini",
	}, zap.NewNop())
	provider.groq.baseURL = groq.URL
	provider.voiceAttemptTimeout = 50 * time.Millisecond

	var chunks []SentenceChunk
	err := provider.ProcessTranscript(context.Background(), TranscriptRequest{
		Transcript: "What features do you have?", Language: "en", MaxTokens: 100,
	}, func(chunk SentenceChunk) { chunks = append(chunks, chunk) })

	require.NoError(t, err)
	require.Len(t, chunks, 1)
	assert.Equal(t, "Here are the CRM features.", chunks[0].Text)
}

func testGeminiProvider(t *testing.T, output string) (*Provider, func()) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprintf(w, "data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":%q}]}}]}\n\n", output)
	}))
	provider := NewProvider(&config.Config{
		GeminiAPIKey:  "test-key",
		GeminiModel:   "gemini-2.5-flash",
		GeminiBaseURL: server.URL,
		LLMProvider:   "gemini",
	}, zap.NewNop())
	return provider, server.Close
}
