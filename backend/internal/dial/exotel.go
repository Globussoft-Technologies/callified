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
	appType    string // "exoml" (legacy XML) or "voicebot" (AgentStream JSON)
	client     *http.Client
}

// NewExotelClient creates an Exotel REST client.
// appType should be "exoml" for legacy ExoML XML flows or "voicebot" for
// modern AgentStream Voicebot flows that expect a JSON dynamic URL response.
func NewExotelClient(apiKey, apiToken, accountSID, callerID, appID, appType string) *ExotelClient {
	if appType == "" {
		appType = "exoml"
	}
	return &ExotelClient{
		apiKey:     apiKey,
		apiToken:   apiToken,
		accountSID: accountSID,
		callerID:   callerID,
		appID:      appID,
		appType:    appType,
		client:     &http.Client{Timeout: 15 * time.Second},
	}
}

// InitiateCall dials toPhone via Exotel Connect API and returns the call SID.
// The dashboard app/flow referenced by appID must route the answered call to
// our WebSocket endpoint:
//   - exoml   (legacy): Passthru applet fetches {PUBLIC_SERVER_URL}/webhook/exotel
//                       and expects XML <Connect><Stream url="..."/>.</Connect>
//   - voicebot (modern): Voicebot applet fetches {PUBLIC_SERVER_URL}/webhook/exotel
//                        and expects JSON {"url":"wss://..."}.
// Per-call context (lead_id, name, phone) is hydrated by wshandler from Redis
// instead of being passed through this URL.
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

	// Modern AgentStream Voicebot flows use /{sid}/exoml/start_voice/{flow_id}.
	// Legacy ExoML apps use /exoml/start/{app_id}.
	if e.appType == "voicebot" {
		exomlURL = fmt.Sprintf("http://my.exotel.com/%s/exoml/start_voice/%s", e.accountSID, e.appID)
	} else {
		exomlURL = fmt.Sprintf("http://my.exotel.com/exoml/start/%s", e.appID)
	}

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

// InitiateHumanCall dials agentPhone first (two-party bridge, no Url).
// Exotel calls the agent; once the agent picks up, Exotel calls customerPhone
// and bridges both parties. Url is intentionally omitted — Exotel overrides
// any custom Url with the App's configured Passthru URL, which would route the
// call into the AI stream instead of bridging the customer. From+To without Url
// is the standard Exotel two-legged call and works reliably.
func (e *ExotelClient) InitiateHumanCall(ctx context.Context, agentPhone, customerPhone, callbackURL string) (string, error) {
	endpoint := fmt.Sprintf(
		"https://api.exotel.com/v1/Accounts/%s/Calls/connect.json",
		e.accountSID)

	form := url.Values{}
	form.Set("From", ExotelPhone(agentPhone))
	form.Set("To", ExotelPhone(customerPhone))
	form.Set("CallerId", e.callerID)
	form.Set("CallType", "trans")
	form.Set("Record", "true")
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
		return "", fmt.Errorf("exotel: human call: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("exotel: status %d: %s", resp.StatusCode, string(body))
	}
	sid := extractNestedJSON(string(body), "Call", "Sid")
	if sid == "" {
		sid = extractJSON(string(body), "Sid")
	}
	if sid == "" {
		return "", fmt.Errorf("exotel: no Sid in response: %s", string(body))
	}
	return sid, nil
}

// Hangup ends an in-progress Exotel call by setting its status to completed.
// This is the carrier-side hang-up used by the browser-to-phone bridge when
// the agent clicks Hang Up in the browser UI.
func (e *ExotelClient) Hangup(ctx context.Context, callSid string) error {
	endpoint := fmt.Sprintf(
		"https://api.exotel.com/v1/Accounts/%s/Calls/%s",
		e.accountSID, callSid)

	form := url.Values{}
	form.Set("Status", "completed")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint,
		strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("exotel: build hangup request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(e.apiKey, e.apiToken)

	resp, err := e.client.Do(req)
	if err != nil {
		return fmt.Errorf("exotel: hangup request: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("exotel: hangup status %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

// FetchRecordingURL fetches call details from Exotel and returns RecordingUrl
// when the call is completed and a recording is available.
// Returns ("", nil) when not ready yet; error only on network/auth failures.
// Exotel stores RecordingUrl in the Call object at /Calls/{sid}.json —
// NOT in the /Calls/{sid}/Recordings.json sub-resource.
func (e *ExotelClient) FetchRecordingURL(ctx context.Context, callSid string) (string, error) {
	endpoint := fmt.Sprintf(
		"https://api.exotel.com/v1/Accounts/%s/Calls/%s.json",
		e.accountSID, callSid)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", err
	}
	req.SetBasicAuth(e.apiKey, e.apiToken)
	resp, err := e.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusNotFound {
		return "", nil
	}
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("exotel call fetch: status %d: %s", resp.StatusCode, string(body))
	}
	// Response: {"Call": {"Status":"completed", "RecordingUrl":"https://...", ...}}
	status := extractNestedJSON(string(body), "Call", "Status")
	if status != "completed" {
		return "", nil // call not finished yet
	}
	recURL := extractNestedJSON(string(body), "Call", "RecordingUrl")
	return recURL, nil
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
