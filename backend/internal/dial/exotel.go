package dial

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// ExotelClient calls the Exotel Connect API.
type ExotelClient struct {
	apiKey     string
	apiToken   string
	accountSID string
	callerID   string
	appID      string
	client     *http.Client
}

// NewExotelClient creates an Exotel REST client.
func NewExotelClient(apiKey, apiToken, accountSID, callerID, appID string) *ExotelClient {
	return &ExotelClient{
		apiKey:     apiKey,
		apiToken:   apiToken,
		accountSID: accountSID,
		callerID:   callerID,
		appID:      appID,
		client:     &http.Client{Timeout: 15 * time.Second},
	}
}

// InitiateCall dials toPhone via Exotel Connect API and returns the call SID.
// exomlURL is ignored — Exotel rejects arbitrary URLs in the Url field; only
// the Exotel-hosted app URL (http://my.exotel.com/exoml/start/{appID}) works.
// The dashboard app referenced by appID must have a Passthru applet pointing
// at {PUBLIC_SERVER_URL}/webhook/exotel — that's what triggers our handler to
// return the WebSocket-streaming ExoML when the lead answers.
// callbackURL receives status events (answered, completed, etc.).
//
// Do NOT send "To" in this flow — Exotel rejects Url + To with 400
// Bad/missing parameters (code 34001).
// CallType=trans matches the working Python implementation; without it the
// dashboard app's Passthru applet is never invoked and the call drops on answer.
func (e *ExotelClient) InitiateCall(ctx context.Context, toPhone, exomlURL, callbackURL string) (string, error) {
	endpoint := fmt.Sprintf(
		"https://api.exotel.com/v1/Accounts/%s/Calls/connect.json",
		e.accountSID)

	// Always use the Exotel-hosted app URL. Custom URLs (even on our own
	// domain) are silently rejected — the call rings, the lead picks up,
	// then drops because Exotel never fetched ExoML. Per-call context
	// (lead_id, name, phone) is hydrated by wshandler from Redis instead
	// of being passed through this URL.
	exomlURL = fmt.Sprintf("http://my.exotel.com/exoml/start/%s", e.appID)

	phone := ExotelPhone(toPhone)
	form := url.Values{}
	form.Set("From", phone)
	form.Set("CallerId", e.callerID)
	form.Set("Url", exomlURL)
	form.Set("CallType", "trans")
	if callbackURL != "" {
		form.Set("StatusCallback", callbackURL)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint,
		strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("exotel: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(e.apiKey, e.apiToken)

	resp, err := e.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("exotel: dial: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	fmt.Printf("[exotel] InitiateCall request: From=%s CallerId=%s Url=%s CallType=trans\n", phone, e.callerID, exomlURL)
	fmt.Printf("[exotel] InitiateCall response status=%d body=%s\n", resp.StatusCode, string(body))

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("exotel: status %d: %s", resp.StatusCode, string(body))
	}
	// Response JSON: {"Call":{"Sid":"...","Status":"in-progress",...}}
	sid := extractNestedJSON(string(body), "Call", "Sid")
	if sid == "" {
		// Fallback: try top-level Sid
		sid = extractJSON(string(body), "Sid")
	}
	if sid == "" {
		return "", fmt.Errorf("exotel: no Sid in response: %s", string(body))
	}
	return sid, nil
}

// NormalizePhone converts an Indian phone number to E.164 format (+91XXXXXXXXXX).
// Handles 10-digit numbers, numbers with spaces/dashes, and numbers already with +91.
func NormalizePhone(phone string) string {
	// Strip whitespace, dashes, parentheses
	phone = strings.Map(func(r rune) rune {
		if r == ' ' || r == '-' || r == '(' || r == ')' || r == '.' {
			return -1
		}
		return r
	}, phone)
	if strings.HasPrefix(phone, "+91") {
		return phone
	}
	if strings.HasPrefix(phone, "91") && len(phone) == 12 {
		return "+" + phone
	}
	if len(phone) == 10 {
		return "+91" + phone
	}
	return phone
}

// ExotelPhone returns the phone in the format Exotel's Connect API accepts:
// "91XXXXXXXXXX" (country code + number, no leading +). 10-digit numbers get
// "91" prefixed. Matches the Python dial_exotel normalisation.
func ExotelPhone(phone string) string {
	phone = strings.TrimSpace(NormalizePhone(phone))
	phone = strings.TrimPrefix(phone, "+")
	if len(phone) == 10 {
		return "91" + phone
	}
	return phone
}
