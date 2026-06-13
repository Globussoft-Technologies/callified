package api

import (
	"context"
	"crypto/sha1"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// hibpClient holds the HTTP client used for the haveibeenpwned k-anonymity
// endpoint. Short timeout because this call is in the password-validation hot
// path on signup / reset / accept-invite — better to skip the breach check
// than to block account creation behind a slow network.
var hibpClient = &http.Client{Timeout: 3 * time.Second}

// hibpBreachThreshold is the minimum appearance count from HIBP that causes us
// to reject a password. 1 would refuse anything that has ever surfaced in any
// breach (very strict; locks out users with otherwise-fine passwords that
// happened to leak once at some random forum). 5 is a pragmatic middle: still
// blocks the long tail of obviously-burned passwords but doesn't penalize a
// user whose passphrase showed up in one obscure breach.
const hibpBreachThreshold = 5

// isPwnedPassword reports whether `password` appears in HIBP's pwned-passwords
// dataset at least `hibpBreachThreshold` times, without sending the password
// itself over the wire (k-anonymity model: send only the first 5 hex chars of
// the SHA-1 hash, scan the response for our remaining 35 chars).
//
// Returns (false, nil) on any network/parse failure — we fail-open so an HIBP
// outage can't lock users out of signup. The caller is expected to log the
// error if non-nil. Don't change this contract without thinking about the
// availability impact.
//
// Why SHA-1? HIBP exposes only the SHA-1 dataset over its k-anonymity range
// API. We're not using SHA-1 to *store* the password — bcrypt does that. The
// SHA-1 hash here exists for the duration of one HTTP call.
func isPwnedPassword(ctx context.Context, password string) (bool, error) {
	if password == "" {
		return false, nil
	}
	sum := sha1.Sum([]byte(password))
	hex := fmt.Sprintf("%X", sum) // 40 uppercase hex chars
	prefix, suffix := hex[:5], hex[5:]

	req, err := http.NewRequestWithContext(ctx,
		http.MethodGet,
		"https://api.pwnedpasswords.com/range/"+prefix, nil)
	if err != nil {
		return false, err
	}
	// Tells HIBP to add a small bit of padding to the response so the exact
	// hash count isn't trivially recoverable from response size. They strip
	// padded entries with `:0` counts; we ignore those naturally.
	req.Header.Set("Add-Padding", "true")
	req.Header.Set("User-Agent", "callified-auth/1")

	resp, err := hibpClient.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("hibp: status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, err
	}

	// Response is one line per match: "<35-hex-suffix>:<count>"
	for _, line := range strings.Split(string(body), "\n") {
		line = strings.TrimSpace(line)
		colon := strings.IndexByte(line, ':')
		if colon != 35 {
			continue
		}
		if !strings.EqualFold(line[:35], suffix) {
			continue
		}
		var count int
		_, _ = fmt.Sscanf(line[colon+1:], "%d", &count)
		return count >= hibpBreachThreshold, nil
	}
	return false, nil
}
