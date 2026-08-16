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
)

// ApplyGuardrails sanitizes LLM output before it reaches TTS or persisted text.
// It preserves [HANGUP], logs policy violations, and returns a cleaned string.
func ApplyGuardrails(text, language string, log *zap.Logger) string {
	if text == "" {
		return ""
	}

	// Preserve [HANGUP] sentinel; re-attach after cleaning.
	hasHangup := strings.Contains(text, "[HANGUP]")
	clean := strings.ReplaceAll(text, "[HANGUP]", "")

	// 1. Strip markdown syntax that TTS reads literally.
	if strings.ContainsAny(clean, "*#_-") {
		clean = stripInlineMarkdown(clean)
		clean = markdownRe.ReplaceAllString(clean, "$2")
	}

	// 2. Redact URLs and email addresses.
	if urlRe.MatchString(clean) {
		clean = urlRe.ReplaceAllString(clean, "[link removed]")
	}

	// 3. Redact phone numbers to prevent accidental leakage.
	if phoneRe.MatchString(clean) {
		clean = phoneRe.ReplaceAllString(clean, "[phone number]")
	}

	// 4. Language mismatch detection: log but do not translate.
	if isIndicLanguage(language) && containsExcessiveLatin(clean) {
		if log != nil {
			log.Warn("guardrails: response contains mostly Latin characters for an Indic language",
				zap.String("language", language),
				zap.String("text_preview", truncate(clean, 80)))
		}
	}

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
