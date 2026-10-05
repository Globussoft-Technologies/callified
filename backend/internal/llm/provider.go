package llm

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode"

	"go.uber.org/zap"

	"github.com/globussoft/callified-backend/internal/config"
)

var ErrMissingSpokenEnvelope = errors.New("llm response missing spoken envelope")
var ErrEmptySpokenResponse = errors.New("llm response contained no spoken text")

const defaultVoiceAttemptTimeout = 7 * time.Second

const voiceOutputProtocol = `

## VOICE OUTPUT PROTOCOL (HIGHEST PRIORITY)
Return exactly one customer-facing spoken reply wrapped in <SAY> and </SAY>.
Example: <SAY>Hello, when are you free for a short demo?</SAY>
Do not output reasoning, analysis, rule names, call-flow commentary, memory commentary, or any text outside <SAY>.
If the call must end, put the literal [HANGUP] immediately before </SAY>.
Only text inside <SAY> is delivered to the customer.`

const voiceActionProtocol = `

Only when the customer explicitly confirms BOTH the appointment day/date AND an exact clock time, declines, or asks to end the call, call the complete_call function. A day such as "today" or "tomorrow" without a clock time is incomplete: ask for the exact time and do not call complete_call. A request to repeat, clarify, or say something again is never a reason to complete the call. Put the short confirmation and goodbye in spoken_text. Do not write a final reply as normal text in that case.`

// Provider routes LLM calls to Gemini or Groq and handles streaming sentence splitting.
// Language "mr" (Marathi) always uses Gemini for better Devanagari support.
// All other languages use LLM_PROVIDER env var (default: gemini).
type Provider struct {
	gemini              *GeminiClient
	groq                *GroqClient
	cfg                 *config.Config
	log                 *zap.Logger
	voiceAttemptTimeout time.Duration
}

// NewProvider creates a Provider wired to Gemini and Groq from cfg.
func NewProvider(cfg *config.Config, log *zap.Logger) *Provider {
	return &Provider{
		gemini:              NewGeminiClient(cfg.GeminiAPIKey, cfg.GeminiModel, cfg.GeminiBaseURL),
		groq:                NewGroqClient(cfg.GroqAPIKey, cfg.GroqModel),
		cfg:                 cfg,
		log:                 log,
		voiceAttemptTimeout: defaultVoiceAttemptTimeout,
	}
}

// ProcessTranscript calls the selected LLM, streams the response, splits it into
// sentences via SplitBuffer, detects [HANGUP], and calls onSentence for each sentence.
// Mirrors Python ws_handler.py _process_transcript LLM section.
func (p *Provider) ProcessTranscript(ctx context.Context, req TranscriptRequest, onSentence func(SentenceChunk)) error {
	req.SystemPrompt += voiceOutputProtocol
	deliver := func(chunk SentenceChunk) {
		onSentence(chunk)
	}

	// All languages follow LLM_PROVIDER config.
	useGemini := p.cfg.LLMProvider != "groq"
	providerName := "groq"
	if useGemini {
		providerName = "gemini"
		req.EnableVoiceActions = true
		req.SystemPrompt += voiceActionProtocol
	}
	p.log.Info("[LLM] processing transcript",
		zap.String("provider", providerName),
		zap.String("language", req.Language),
		zap.Int32("max_tokens", req.MaxTokens),
	)

	result, lastErr := p.processVoiceProvider(ctx, useGemini, providerName, req, deliver, 2)
	if result.delivered {
		return result.err
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}

	// A Gemini gateway may accept the HTTP connection but never emit an SSE
	// event. Fall back only if no text was delivered, avoiding duplicate or
	// contradictory speech after a partial response.
	if useGemini && p.groq != nil && strings.TrimSpace(p.groq.apiKey) != "" {
		p.log.Warn("[LLM] Gemini unavailable; falling back to Groq",
			zap.String("language", req.Language),
			zap.Error(lastErr),
		)
		fallbackReq := req
		fallbackReq.EnableVoiceActions = false
		fallbackReq.SystemPrompt += `

For this fallback request the complete_call function is unavailable. If the call must end, put the literal [HANGUP] immediately before </SAY>.`
		result, fallbackErr := p.processVoiceProvider(ctx, false, "groq_fallback", fallbackReq, deliver, 1)
		if result.delivered {
			return result.err
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if fallbackErr != nil {
			lastErr = fallbackErr
		}
	}

	// Release the turn with a localized prompt even if all providers fail. This
	// lets queued customer speech continue through the sequential call pipeline.
	deliver(SentenceChunk{Text: safeVoiceRetry(req.Language)})
	if lastErr == nil {
		return ErrEmptySpokenResponse
	}
	return lastErr
}

func (p *Provider) processVoiceProvider(ctx context.Context, useGemini bool, providerName string, req TranscriptRequest, deliver func(SentenceChunk), maxAttempts int) (voiceAttemptResult, error) {
	var lastErr error
	var result voiceAttemptResult
	for attempt := 0; attempt < maxAttempts; attempt++ {
		attemptReq := req
		if attempt > 0 {
			attemptReq.SystemPrompt += `

Your previous response could not be delivered because it did not contain a non-empty <SAY>...</SAY> reply. Respond again now and follow the VOICE OUTPUT PROTOCOL exactly.`
			p.log.Warn("[LLM] retrying invalid voice envelope",
				zap.String("provider", providerName),
				zap.String("language", req.Language),
				zap.Error(lastErr),
			)
		}

		attemptCtx, cancel := context.WithTimeout(ctx, p.voiceAttemptTimeout)
		result = p.streamVoiceAttempt(attemptCtx, useGemini, attemptReq, deliver)
		cancel()
		lastErr = result.err
		if result.delivered {
			return result, result.err
		}
		if ctx.Err() != nil {
			return result, ctx.Err()
		}
		if errors.Is(result.err, context.DeadlineExceeded) || errors.Is(result.err, context.Canceled) {
			lastErr = context.DeadlineExceeded
			p.log.Warn("[LLM] live provider attempt timed out",
				zap.String("provider", providerName),
				zap.String("language", req.Language),
				zap.Duration("timeout", p.voiceAttemptTimeout),
			)
			break
		}
		if result.invalidEnvelope == nil {
			break
		}
		lastErr = result.invalidEnvelope
	}
	return result, lastErr
}

type voiceAttemptResult struct {
	err             error
	invalidEnvelope error
	delivered       bool
}

func (p *Provider) streamVoiceAttempt(ctx context.Context, useGemini bool, req TranscriptRequest, deliver func(SentenceChunk)) voiceAttemptResult {
	var buf strings.Builder
	var raw strings.Builder
	metaFilter := newBracketedMetaFilter()
	spokenFilter := newSpokenEnvelopeFilter()
	result := voiceAttemptResult{}
	actionHandled := false
	textDeliveredBeforeAction := false
	deliverText := func(chunk SentenceChunk) {
		if strings.TrimSpace(chunk.Text) != "" {
			textDeliveredBeforeAction = true
		}
		deliver(chunk)
	}
	req.OnVoiceAction = func(action VoiceAction) {
		if actionHandled || action.Name != "complete_call" {
			return
		}
		actionHandled = true
		spoken := ""
		if !textDeliveredBeforeAction {
			spoken = collapseExactRepeatedVoiceText(sanitizeUntaggedVoiceResponse(action.SpokenText))
			if spoken == "" {
				spoken = safeFinalGoodbye(req.Language)
			}
		}
		// If the model streamed normal customer-facing text and then emitted the
		// terminal function, the function is control-only. Replaying spoken_text
		// here would speak and persist the same goodbye twice. The pipeline still
		// receives the outcome and can validate or reject the terminal action.
		result.delivered = true
		deliver(SentenceChunk{Text: spoken, HasHangup: true, HangupOutcome: action.Outcome})
	}

	onToken := func(token string) {
		if actionHandled {
			return
		}
		raw.WriteString(token)
		// This is the hard trust boundary for the voice path. Model text before
		// or after <SAY> may contain reasoning and is never allowed into TTS.
		token = spokenFilter.Write(token)
		if token == "" {
			return
		}
		token = metaFilter.Write(token)
		if token == "" {
			return
		}
		buf.WriteString(token)
		sentences, remainder := SplitBuffer(buf.String())
		buf.Reset()
		buf.WriteString(remainder)
		for _, sent := range sentences {
			if text, hangup := parseChunk(sent); text != "" || hangup {
				result.delivered = result.delivered || text != ""
				deliverText(SentenceChunk{Text: text, HasHangup: hangup})
			}
		}
	}

	if useGemini {
		result.err = p.gemini.StreamTokens(ctx, req, onToken)
	} else {
		result.err = p.groq.StreamTokens(ctx, req, onToken)
	}
	if actionHandled {
		return result
	}
	if !spokenFilter.SawEnvelope() && (result.err == nil || errors.Is(result.err, ErrMaxTokens)) {
		// Tags are a preferred model-output convention, not a single point of
		// failure for a live call. If the completed untagged response is clearly
		// customer-facing, deliver it after stripping known control narration.
		// Truncated output is never accepted through this compatibility path.
		if result.err == nil {
			if safe := sanitizeUntaggedVoiceResponse(raw.String()); safe != "" {
				deliverCompleteVoiceText(safe, deliver, &result)
				if result.delivered {
					return result
				}
			}
		}
		result.invalidEnvelope = ErrMissingSpokenEnvelope
		return result
	}

	// Flush any text left in the buffer after stream ends (no trailing punctuation).
	// A closing </SAY> proves the response is complete, so an otherwise valid
	// unpunctuated reply is safe even for inbound calls. Without a closing tag,
	// retain the previous protection against truncated realtime speech.
	completeEnvelope := spokenFilter.ClosedEnvelope()
	canFlushRemainder := completeEnvelope || (!errors.Is(result.err, ErrMaxTokens) && !req.DropIncompleteRemainder)
	if remaining := strings.TrimSpace(buf.String()); remaining != "" && canFlushRemainder {
		text, hangup := parseChunk(remaining)
		if text != "" || hangup {
			result.delivered = result.delivered || text != ""
			deliverText(SentenceChunk{Text: text, HasHangup: hangup})
		}
	}
	if !result.delivered && result.err == nil {
		result.invalidEnvelope = ErrEmptySpokenResponse
	}
	return result
}

// collapseExactRepeatedVoiceText protects the last output boundary from a
// model returning "confirmation goodbye confirmation goodbye" inside a single
// structured spoken_text argument. It only collapses two identical, reasonably
// long halves; ordinary emphasis such as "yes, yes" is preserved.
func collapseExactRepeatedVoiceText(text string) string {
	text = strings.TrimSpace(text)
	if len(text) < 40 {
		return text
	}
	for i := len(text) / 3; i <= len(text)*2/3; i++ {
		if i >= len(text) || (text[i] != ' ' && text[i] != '\n' && text[i] != '\t') {
			continue
		}
		left := strings.TrimSpace(text[:i])
		right := strings.TrimSpace(text[i:])
		if len(left) >= 20 && strings.EqualFold(left, right) {
			return left
		}
	}
	return text
}

func deliverCompleteVoiceText(text string, deliver func(SentenceChunk), result *voiceAttemptResult) {
	sentences, remainder := SplitBuffer(text)
	if remaining := strings.TrimSpace(remainder); remaining != "" {
		sentences = append(sentences, remaining)
	}
	for _, sentence := range sentences {
		if spoken, hangup := parseChunk(sentence); spoken != "" || hangup {
			result.delivered = result.delivered || spoken != ""
			deliver(SentenceChunk{Text: spoken, HasHangup: hangup})
		}
	}
}

// sanitizeUntaggedVoiceResponse is the compatibility path for models that
// return a normal spoken answer but omit the requested XML envelope. Known
// analysis/control prefixes are removed; analysis-only output is rejected.
func sanitizeUntaggedVoiceResponse(text string) string {
	text = strings.TrimSpace(text)
	text = strings.TrimPrefix(text, "```text")
	text = strings.TrimPrefix(text, "```")
	text = strings.TrimSuffix(text, "```")
	text = stripVoiceEnvelopeTags(text)
	text = stripBracketedMeta(strings.TrimSpace(text))
	if text == "" {
		return ""
	}
	if containsControlNarration(text) {
		text = stripLeakedControlNarration(text)
	}
	if text == "" || containsControlNarration(text) {
		return ""
	}
	return strings.TrimSpace(text)
}

// spokenEnvelopeFilter releases only text inside the first <SAY>...</SAY>
// envelope. Tags may be split across arbitrary streaming token boundaries.
type spokenEnvelopeFilter struct {
	pending strings.Builder
	inside  bool
	done    bool
	sawOpen bool
}

func newSpokenEnvelopeFilter() *spokenEnvelopeFilter { return &spokenEnvelopeFilter{} }

func (f *spokenEnvelopeFilter) SawEnvelope() bool { return f.sawOpen }

func (f *spokenEnvelopeFilter) ClosedEnvelope() bool { return f.done }

func (f *spokenEnvelopeFilter) Write(token string) string {
	if f.done || token == "" {
		return ""
	}
	f.pending.WriteString(token)
	var out strings.Builder
	for {
		value := f.pending.String()
		if !f.inside {
			open := indexFold(value, "<say>")
			if open < 0 {
				// Retain only a possible partial tag beginning with '<'. All
				// complete text outside the envelope is intentionally discarded.
				if last := strings.LastIndexByte(value, '<'); last >= 0 {
					f.setPending(value[last:])
				} else {
					f.pending.Reset()
				}
				return out.String()
			}
			f.sawOpen = true
			f.inside = true
			f.setPending(value[open+len("<say>"):])
			continue
		}

		closeAt := indexFold(value, "</say>")
		if closeAt >= 0 {
			out.WriteString(value[:closeAt])
			f.pending.Reset()
			f.inside = false
			f.done = true
			return out.String()
		}
		// Do not emit a trailing '<...': it may be a closing tag split over
		// multiple SSE events. Everything before it is safe spoken text.
		if last := strings.LastIndexByte(value, '<'); last >= 0 {
			out.WriteString(value[:last])
			f.setPending(value[last:])
		} else {
			out.WriteString(value)
			f.pending.Reset()
		}
		return out.String()
	}
}

func (f *spokenEnvelopeFilter) setPending(value string) {
	f.pending.Reset()
	f.pending.WriteString(value)
}

func indexFold(value, target string) int {
	for offset := 0; offset < len(value); {
		rel := strings.IndexByte(value[offset:], '<')
		if rel < 0 {
			return -1
		}
		start := offset + rel
		if len(value)-start >= len(target) && strings.EqualFold(value[start:start+len(target)], target) {
			return start
		}
		offset = start + 1
	}
	return -1
}

func safeVoiceRetry(language string) string {
	switch language {
	case "hi":
		return "माफ़ कीजिए, क्या आप एक बार फिर कहेंगे?"
	case "mr":
		return "माफ करा, तुम्ही पुन्हा एकदा सांगाल का?"
	case "bn":
		return "দুঃখিত, আরেকবার বলবেন?"
	case "gu":
		return "માફ કરશો, ફરી એક વાર કહેશો?"
	case "pa":
		return "ਮਾਫ਼ ਕਰਨਾ, ਕੀ ਤੁਸੀਂ ਇੱਕ ਵਾਰ ਫਿਰ ਦੱਸੋਗੇ?"
	case "ta":
		return "மன்னிக்கவும், இன்னொரு முறை சொல்ல முடியுமா?"
	case "te":
		return "క్షమించండి, ఇంకొకసారి చెబుతారా?"
	case "kn":
		return "ಕ್ಷಮಿಸಿ, ಇನ್ನೊಮ್ಮೆ ಹೇಳುತ್ತೀರಾ?"
	case "ml":
		return "ക്ഷമിക്കണം, ഒരിക്കൽ കൂടി പറയാമോ?"
	default:
		return "Sorry, could you please say that again?"
	}
}

func safeFinalGoodbye(language string) string {
	switch language {
	case "hi":
		return "धन्यवाद। आपका दिन शुभ हो।"
	case "mr":
		return "धन्यवाद। तुमचा दिवस चांगला जावो।"
	case "bn":
		return "ধন্যবাদ। আপনার দিনটি শুভ হোক।"
	case "gu":
		return "આભાર। તમારો દિવસ શુભ રહે।"
	case "pa":
		return "ਧੰਨਵਾਦ। ਤੁਹਾਡਾ ਦਿਨ ਚੰਗਾ ਰਹੇ।"
	case "ta":
		return "நன்றி। உங்கள் நாள் இனிதாக அமையட்டும்।"
	case "te":
		return "ధన్యవాదాలు। మీ రోజు శుభంగా ఉండాలి।"
	case "kn":
		return "ಧನ್ಯವಾದಗಳು। ನಿಮ್ಮ ದಿನ ಶುಭವಾಗಿರಲಿ।"
	case "ml":
		return "നന്ദി। നിങ്ങളുടെ ദിവസം നല്ലതാകട്ടെ।"
	default:
		return "Thank you for your time. Have a good day."
	}
}

type bracketedMetaFilter struct {
	inBracket bool
	pending   strings.Builder
}

func newBracketedMetaFilter() *bracketedMetaFilter {
	return &bracketedMetaFilter{}
}

func (f *bracketedMetaFilter) Write(token string) string {
	var out strings.Builder
	for _, r := range token {
		if f.inBracket {
			f.pending.WriteRune(r)
			if r == ']' {
				pending := f.pending.String()
				if strings.EqualFold(strings.TrimSpace(pending), "[HANGUP]") {
					out.WriteString("[HANGUP]")
				}
				f.pending.Reset()
				f.inBracket = false
			}
			continue
		}
		if r == '[' {
			f.inBracket = true
			f.pending.Reset()
			f.pending.WriteRune(r)
			continue
		}
		out.WriteRune(r)
	}
	return out.String()
}

// ClassifyRepeatIntent returns a stable semantic key for repeated-question
// detection. The key is generated by the LLM so semantically identical
// questions in different languages can match without a hardcoded intent list.
func (p *Provider) ClassifyRepeatIntent(ctx context.Context, text, language string) (string, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return "", nil
	}
	systemPrompt := strings.TrimSpace(`
You classify one customer utterance for repeated-question detection in an outbound sales call.
Return only one short snake_case semantic intent key, or "none".
Use the same key when the customer asks the same meaning in any language.
Return "none" for acknowledgements, filler sounds, greetings, or plain answers.
Do not explain. Do not add punctuation. Do not include a fixed category unless it captures the actual meaning.
`)
	history := []ChatMessage{{
		Role: "user",
		Text: "Language: " + language + "\nCustomer: " + text,
	}}
	resp, err := p.GenerateResponse(ctx, systemPrompt, history, 32)
	if err != nil {
		return "", err
	}
	return sanitizeRepeatIntentKey(resp), nil
}

func sanitizeRepeatIntentKey(text string) string {
	text = strings.ToLower(strings.TrimSpace(text))
	text = strings.Trim(text, "`\"' .,:;!?()[]{}")
	var b strings.Builder
	lastUnderscore := false
	for _, r := range text {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
			lastUnderscore = false
		case r == '_' || unicode.IsSpace(r) || r == '-' || r == '/':
			if !lastUnderscore && b.Len() > 0 {
				b.WriteByte('_')
				lastUnderscore = true
			}
		}
		if b.Len() >= 64 {
			break
		}
	}
	key := strings.Trim(b.String(), "_")
	switch key {
	case "", "none", "no_intent", "unknown", "filler", "greeting", "acknowledgement", "acknowledgment":
		return ""
	default:
		return key
	}
}

// GenerateResponse calls the LLM without streaming and returns the full reply.
// Used by WA agent, prompt generation, and other non-real-time contexts.
func (p *Provider) GenerateResponse(ctx context.Context, systemPrompt string, history []ChatMessage, maxTokens int32) (string, error) {
	req := TranscriptRequest{
		Transcript:   "", // will use last message from history
		SystemPrompt: systemPrompt,
		History:      history,
		MaxTokens:    maxTokens,
	}
	// Extract the last user message as the transcript
	for i := len(history) - 1; i >= 0; i-- {
		if history[i].Role == "user" {
			req.Transcript = history[i].Text
			req.History = history[:i]
			break
		}
	}

	useGemini := p.cfg.LLMProvider != "groq"
	var result strings.Builder
	onToken := func(t string) { result.WriteString(t) }

	var err error
	if useGemini {
		err = p.gemini.StreamTokens(ctx, req, onToken)
	} else {
		err = p.groq.StreamTokens(ctx, req, onToken)
	}
	return strings.TrimSpace(result.String()), err
}

// GenerateText calls Gemini (non-streaming) with thinking disabled.
// Suitable for batch extraction tasks like product scraping and prompt generation.
func (p *Provider) GenerateText(ctx context.Context, systemPrompt, userMessage string, maxOutputTokens int) (string, error) {
	return p.gemini.GenerateText(ctx, systemPrompt, userMessage, maxOutputTokens)
}

// parseChunk strips [HANGUP] from text and returns (cleanText, hasHangup).
func parseChunk(text string) (string, bool) {
	hasHangup := strings.Contains(text, "[HANGUP]")
	clean := strings.TrimSpace(strings.ReplaceAll(text, "[HANGUP]", ""))
	clean = stripVoiceEnvelopeTags(clean)
	clean = stripBracketedMeta(clean)
	clean = stripLeakedControlNarration(clean)
	return clean, hasHangup
}

// stripVoiceEnvelopeTags removes protocol markers at the final shared output
// boundary. This catches orphan or malformed-by-position tags that bypass the
// preferred streaming envelope parser; protocol syntax must never reach TTS or
// transcript storage.
func stripVoiceEnvelopeTags(text string) string {
	var out strings.Builder
	for len(text) > 0 {
		open := strings.IndexByte(text, '<')
		if open < 0 {
			out.WriteString(text)
			break
		}
		out.WriteString(text[:open])
		closeRel := strings.IndexByte(text[open:], '>')
		if closeRel < 0 {
			out.WriteString(text[open:])
			break
		}
		closeAt := open + closeRel
		name := strings.Map(func(r rune) rune {
			if unicode.IsSpace(r) {
				return -1
			}
			return unicode.ToLower(r)
		}, text[open+1:closeAt])
		if name != "say" && name != "/say" {
			out.WriteString(text[open : closeAt+1])
		}
		text = text[closeAt+1:]
	}
	return strings.TrimSpace(out.String())
}

// stripBracketedMeta removes non-spoken model side-notes such as
// "[The user said ...]" that occasionally leak from the LLM. The voice path
// must speak only customer-facing text; [HANGUP] is handled before this runs.
func stripBracketedMeta(text string) string {
	for {
		start := strings.Index(text, "[")
		if start == -1 {
			return strings.TrimSpace(text)
		}
		end := strings.Index(text[start:], "]")
		if end == -1 {
			if start == 0 {
				return ""
			}
			return strings.TrimSpace(text[:start])
		}
		end += start
		text = strings.TrimSpace(text[:start] + " " + text[end+1:])
	}
}

func stripLeakedControlNarration(text string) string {
	text = strings.TrimSpace(text)
	text = strings.TrimLeft(text, ": \t\r\n]")
	if !looksLikeControlNarration(text) {
		return text
	}

	lower := strings.ToLower(text)
	for _, anchor := range []string{
		"i will keep it simple.",
		"i need to re-ask it.",
		"i need to re-ask.",
		"before moving to the next step in the call flow.",
		"the next step in the call flow.",
		"call flow.",
	} {
		if idx := strings.Index(lower, anchor); idx >= 0 {
			if rest := cleanControlNarrationRemainder(text[idx+len(anchor):]); rest != "" {
				return rest
			}
		}
	}

	for _, boundary := range []string{". ", "] "} {
		searchFrom := 0
		for {
			idx := strings.Index(text[searchFrom:], boundary)
			if idx < 0 {
				break
			}
			cut := searchFrom + idx + len(boundary)
			rest := cleanControlNarrationRemainder(text[cut:])
			if rest != "" && !looksLikeControlNarration(rest) {
				return rest
			}
			searchFrom = cut
		}
	}
	return ""
}

func cleanControlNarrationRemainder(text string) string {
	text = strings.TrimSpace(text)
	text = strings.TrimLeft(text, ": \t\r\n]")
	return strings.TrimSpace(text)
}

func looksLikeControlNarration(text string) bool {
	text = strings.TrimSpace(strings.TrimLeft(text, ": \t\r\n]"))
	if text == "" {
		return false
	}
	lower := strings.ToLower(text)
	prefixes := []string{
		"the user",
		"user",
		"the customer",
		"customer",
		"the agent",
		"agent",
		"i need to",
		"i should",
		"i will ask",
		"this means",
		"this is an affirmative",
		"it seems",
	}
	hasPrefix := false
	for _, prefix := range prefixes {
		if strings.HasPrefix(lower, prefix) {
			hasPrefix = true
			break
		}
	}
	if !hasPrefix {
		return false
	}
	markers := []string{
		"user said",
		"user keeps saying",
		"customer's response",
		"customer response",
		"customer said",
		"customer interrupted",
		"does not directly answer",
		"previous question",
		"current question",
		"qualifying question",
		"call flow",
		"next step",
		"according to the",
		"affirmative signal",
		"i should",
		"i will ask",
		"re-ask",
		"unclear",
	}
	for _, marker := range markers {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func containsControlNarration(text string) bool {
	if looksLikeControlNarration(text) {
		return true
	}
	lower := strings.ToLower(text)
	markers := []string{
		"the user said",
		"the user keeps saying",
		"the customer's response",
		"the customer said",
		"according to the forward signal",
		"according to the call flow",
		"i should proceed",
		"i should move to",
		"i need to re-ask",
		"the next step is",
		"this means the question",
	}
	for _, marker := range markers {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}
