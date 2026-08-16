package llm

import (
	"regexp"
	"strings"
	"unicode"

	"go.uber.org/zap"
)

var (
	// markdownRe removes common markdown formatting that TTS would read literally.
	markdownRe = regexp.MustCompile(`(?m)^\s*(#{1,6}\s+|\*\s+|\-\s+|\d+\.\s+|[\*\_]{1,2})(.+)$`)
	// urlRe redacts URLs and email-like patterns.
	urlRe = regexp.MustCompile(`https?://[^\s\]\)<>]+|www\.[^\s\]\)<>]+|[\w.\-]+@[\w.\-]+\.[\w]{2,}`)
	// phoneRe redacts obvious Indian phone numbers (10 digits, optionally +91 or 0 prefix).
	phoneRe = regexp.MustCompile(`(?:\+91|0)?\s*\d{5}\s*\d{5}`)
	// tagRe matches bracket tags like [HANGUP], [HOLD], [MUTE]. Only [HANGUP] is allowed.
	tagRe = regexp.MustCompile(`\[[A-Za-z_][A-Za-z0-9_\-]*\]`)
	// greetingPatterns matches common standalone greetings in English and Indic scripts.
	// Each pattern is anchored to the start of the text and optionally followed by punctuation/spaces.
	greetingPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)^\s*(hello|hi|hey|good\s+(morning|afternoon|evening))\s*[.,!?]*\s*`),
		regexp.MustCompile(`(?i)^\s*(namaste|namaskar|namskaar|salaam|sat\s+sri\s+akal)\s*[.,!?]*\s*`),
		regexp.MustCompile(`^\s*(नमस्ते|नमस्कार|नमस्कार\.\s|सत\s+श्री\s+अकाल|सलाम|हाय|हैलो)\s*[।,.!?]*\s*`),
		regexp.MustCompile(`^\s*(নমস্কার|নমস্তে|হ্যালো|হাই|আসসালামু)\s*[।,.!?]*\s*`),
		regexp.MustCompile(`^\s*(ನಮಸ್ತೆ|ನಮಸ್ಕಾರ|ಹಾಯ್|ಹೆಲೋ)\s*[।,.!?]*\s*`),
		regexp.MustCompile(`^\s*(నమస్తే|నమస్కారం|హాయ్|హలో)\s*[।,.!?]*\s*`),
		regexp.MustCompile(`^\s*(നമസ്കാരം|ഹലോ|ഹായ്)\s*[।,.!?]*\s*`),
		regexp.MustCompile(`^\s*(வணக்கம்|ஹலோ|ஹாய்)\s*[।,.!?]*\s*`),
		regexp.MustCompile(`^\s*(નમસ્તે|નમસ્કાર|હેલો|હાય)\s*[।,.!?]*\s*`),
		regexp.MustCompile(`^\s*(ਨਮਸਤੇ|ਨਮਸਕਾਰ|ਹੈਲੋ|ਹਾਇ)\s*[।,.!?]*\s*`),
		regexp.MustCompile(`^\s*(नमस्कार|नमस्ते|हॅलो|हाय)\s*[।,.!?]*\s*`),
	}
)

// maxResponseChars returns the hard character limit for the final TTS text.
// Indic scripts are more compact per token but tokenize more heavily, so we give
// a slightly larger budget than English.
func maxResponseChars(language string) int {
	if isIndicLanguage(language) {
		return 240
	}
	return 200
}

// ApplyGuardrails sanitizes LLM output before it reaches TTS or persisted text.
// It preserves [HANGUP], logs policy violations, and returns a cleaned string.
// Backward-compatible wrapper that assumes the greeting has not been delivered yet.
func ApplyGuardrails(text, language string, log *zap.Logger) string {
	return ApplyGuardrailsWithState(text, language, false, log)
}

// ApplyGuardrailsWithState sanitizes LLM output with awareness of the call's
// conversation state. When greetingDone is true, any standalone greeting at the
// start of the response is stripped to prevent the AI from re-greeting the user.
func ApplyGuardrailsWithState(text, language string, greetingDone bool, log *zap.Logger) string {
	if text == "" {
		return ""
	}

	// Preserve [HANGUP] sentinel; re-attach after cleaning.
	hasHangup := strings.Contains(text, "[HANGUP]")
	clean := strings.ReplaceAll(text, "[HANGUP]", "")

	// 1. Strip disallowed bracket tags (e.g. [HOLD], [MUTE], [WAIT]) before
	// anything else so they cannot be preserved by later steps.
	if tagRe.MatchString(clean) {
		clean = tagRe.ReplaceAllStringFunc(clean, func(tag string) string {
			if tag == "[HANGUP]" {
				return tag
			}
			if log != nil {
				log.Warn("guardrails: removed disallowed bracket tag",
					zap.String("tag", tag),
					zap.String("language", language))
			}
			return ""
		})
	}

	// 2. Strip markdown syntax that TTS reads literally.
	if strings.ContainsAny(clean, "*#_-`") {
		clean = stripInlineMarkdown(clean)
		clean = markdownRe.ReplaceAllString(clean, "$2")
		clean = stripBackticks(clean)
	}

	// 3. Redact URLs and email addresses.
	if urlRe.MatchString(clean) {
		clean = urlRe.ReplaceAllString(clean, "[link removed]")
	}

	// 4. Redact phone numbers to prevent accidental leakage.
	if phoneRe.MatchString(clean) {
		clean = phoneRe.ReplaceAllString(clean, "[phone number]")
	}

	// 5. Strip repeated greetings when the opening greeting has already been delivered.
	if greetingDone {
		before := clean
		clean = stripGreeting(clean)
		if clean != before && log != nil {
			log.Warn("guardrails: stripped repeated greeting",
				zap.String("language", language),
				zap.String("text_preview", truncate(before, 80)))
		}
	}

	// 6. Language mismatch detection: log but do not translate.
	if isIndicLanguage(language) && containsExcessiveLatin(clean) {
		if log != nil {
			log.Warn("guardrails: response contains mostly Latin characters for an Indic language",
				zap.String("language", language),
				zap.String("text_preview", truncate(clean, 80)))
		}
	}

	// 7. Enforce response length limit, preserving [HANGUP] at the end.
	clean = enforceLengthLimit(clean, language, hasHangup)

	// 8. Normalize whitespace left behind by removed tags/markdown.
	clean = normalizeWhitespace(clean)

	clean = strings.TrimSpace(clean)
	if hasHangup && !strings.HasSuffix(clean, "[HANGUP]") {
		if clean != "" {
			clean += " [HANGUP]"
		} else {
			clean = "[HANGUP]"
		}
	}
	return clean
}

func isIndicLanguage(language string) bool {
	switch language {
	case "hi", "mr", "bn", "gu", "pa", "ta", "te", "kn", "ml":
		return true
	}
	return false
}

// containsExcessiveLatin returns true when the non-whitespace runes are mostly
// Latin letters, excluding common English words that naturally mix into Indic speech.
func containsExcessiveLatin(text string) bool {
	text = strings.TrimSpace(text)
	if text == "" {
		return false
	}
	var latin, total int
	for _, r := range text {
		if unicode.IsSpace(r) || unicode.IsPunct(r) || unicode.IsDigit(r) {
			continue
		}
		total++
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
			latin++
		}
	}
	if total == 0 {
		return false
	}
	// Allow short English phrases like "OK", "sorry", "thank you".
	if total <= 12 {
		return false
	}
	return float64(latin)/float64(total) > 0.75
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// stripGreeting removes a standalone greeting from the beginning of the text.
func stripGreeting(text string) string {
	for _, re := range greetingPatterns {
		if re.MatchString(text) {
			return re.ReplaceAllString(text, "")
		}
	}
	return text
}

// enforceLengthLimit truncates text to the language-specific character budget,
// trying to end on a sentence boundary and falling back to a word boundary.
func enforceLengthLimit(text, language string, hasHangup bool) string {
	limit := maxResponseChars(language)
	if len(text) <= limit {
		return text
	}

	// Try to truncate at the last sentence boundary before the limit.
	cut := text[:limit]
	if idx := strings.LastIndexAny(cut, ".!?।"); idx > 0 {
		// Ensure the boundary is not just an abbreviation or number.
		if !isBoundaryFalsePositive(cut, idx) {
			return strings.TrimSpace(text[:idx+1])
		}
	}

	// Fallback: truncate at the last whitespace before the limit.
	if idx := strings.LastIndexFunc(cut, unicode.IsSpace); idx > 0 {
		return strings.TrimSpace(text[:idx])
	}

	// Hard truncate at the limit as a last resort.
	return strings.TrimSpace(text[:limit])
}

// isBoundaryFalsePositive returns true if the punctuation at idx is likely part
// of an abbreviation or numeric value rather than a sentence end.
func isBoundaryFalsePositive(text string, idx int) bool {
	if idx <= 0 || idx+1 >= len(text) {
		return false
	}
	prev := text[idx-1]
	next := text[idx+1]
	if prev >= '0' && prev <= '9' {
		return true
	}
	if (prev >= 'A' && prev <= 'Z') || (prev >= 'a' && prev <= 'z') {
		// Could be an abbreviation; conservatively treat as false positive if
		// next char is not a space or end of string.
		if next != ' ' && next != '\n' && next != '\t' {
			return true
		}
	}
	return false
}

// stripInlineMarkdown removes paired inline markdown markers (**...**, *...*,
// __...__, _..._) while preserving the text between them. Go's regexp package
// does not support backreferences, so this is implemented with string scanning.
func stripInlineMarkdown(s string) string {
	changed := true
	for changed {
		changed = false
		for _, pair := range []string{"**", "__", "*", "_"} {
			start := strings.Index(s, pair)
			if start < 0 {
				continue
			}
			after := s[start+len(pair):]
			end := strings.Index(after, pair)
			if end < 0 {
				continue
			}
			end += start + len(pair)
			s = s[:start] + s[start+len(pair):end] + s[end+len(pair):]
			changed = true
			break
		}
	}
	return s
}

// stripBackticks removes inline code backticks (`...`) while preserving the code
// text itself. TTS would otherwise read "backtick" literally.
func stripBackticks(s string) string {
	changed := true
	for changed {
		changed = false
		start := strings.Index(s, "`")
		if start < 0 {
			continue
		}
		after := s[start+1:]
		end := strings.Index(after, "`")
		if end < 0 {
			continue
		}
		end += start + 1
		s = s[:start] + s[start+1:end] + s[end+1:]
		changed = true
	}
	return s
}

// normalizeWhitespace collapses runs of spaces/tabs/newlines to a single space
// and trims leading/trailing whitespace. It preserves sentence-ending punctuation.
func normalizeWhitespace(s string) string {
	var b strings.Builder
	inSpace := true
	for _, r := range s {
		if unicode.IsSpace(r) {
			if !inSpace {
				b.WriteByte(' ')
				inSpace = true
			}
			continue
		}
		inSpace = false
		b.WriteRune(r)
	}
	return strings.TrimSpace(b.String())
}
