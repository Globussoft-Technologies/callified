package dial

import (
	"strconv"
	"strings"
)

// extractJSON extracts a string value from a flat JSON object by key.
// Used to avoid importing encoding/json in hot-path client code.
// For example: extractJSON(`{"sid":"CA123","status":"queued"}`, "sid") → "CA123"
func extractJSON(body, key string) string {
	needle := `"` + key + `"`
	idx := strings.Index(strings.ToLower(body), strings.ToLower(needle))
	if idx < 0 {
		return ""
	}
	rest := body[idx+len(needle):]
	rest = strings.TrimSpace(rest)
	if len(rest) == 0 || rest[0] != ':' {
		return ""
	}
	rest = strings.TrimSpace(rest[1:])
	if len(rest) == 0 {
		return ""
	}
	if rest[0] == '"' {
		// Scan for the closing quote, skipping backslash-escaped characters.
		// This correctly handles JSON escape sequences like \/ \\ \" \n etc.
		end := -1
		for i := 1; i < len(rest); i++ {
			if rest[i] == '\\' {
				i++ // skip the escaped character
				continue
			}
			if rest[i] == '"' {
				end = i
				break
			}
		}
		if end < 0 {
			return ""
		}
		// strconv.Unquote handles Go string escapes but NOT JSON's \/ sequence.
		// Normalise \/ → / first so Exotel URLs parse correctly.
		normalized := strings.ReplaceAll(rest[:end+1], `\/`, `/`)
		if unquoted, err := strconv.Unquote(normalized); err == nil {
			return unquoted
		}
		return strings.ReplaceAll(rest[1:end], `\/`, `/`)
	}
	// numeric / bare value
	end := strings.IndexAny(rest, ",}")
	if end < 0 {
		return strings.TrimSpace(rest)
	}
	return strings.TrimSpace(rest[:end])
}

// extractNestedJSON extracts a value from a singly-nested JSON object.
// e.g. extractNestedJSON(`{"Call":{"Sid":"CA1",...}}`, "Call", "Sid") → "CA1"
func extractNestedJSON(body, outer, inner string) string {
	outerKey := `"` + outer + `"`
	idx := strings.Index(body, outerKey)
	if idx < 0 {
		return ""
	}
	start := strings.Index(body[idx:], "{")
	if start < 0 {
		return ""
	}
	sub := body[idx+start:]
	end := strings.Index(sub, "}")
	if end < 0 {
		return ""
	}
	return extractJSON(sub[:end+1], inner)
}
