package dial

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var ErrTataStreamNotReady = errors.New("update and verify the stream secret in Tata before placing calls")

const defaultTataClickToCallEndpoint = "https://api-smartflo.tatateleservices.com/v1/click_to_call_support"
const defaultTataHangupEndpoint = "https://api-smartflo.tatateleservices.com/v1/call/hangup"

// TataClient calls Tata Tele Smartflo/CloudPhone APIs.
//
// The first supported path is Smartflo Click-to-Call. Real-time AI calls also
// need Tata VOICE Streaming or SIP enabled for the account; once Tata provides
// the exact stream payloads, the media handler can be mapped separately.
type TataClient struct {
	apiKey      string
	apiToken    string
	callerID    string
	agentNumber string
	endpoint    string
	client      *http.Client
}

func NewTataClient(apiKey, apiToken, callerID, agentNumber, endpoint string) *TataClient {
	if endpoint == "" {
		endpoint = defaultTataClickToCallEndpoint
	}
	return &TataClient{
		apiKey:      strings.TrimSpace(apiKey),
		apiToken:    strings.TrimSpace(apiToken),
		callerID:    strings.TrimSpace(callerID),
		agentNumber: strings.TrimSpace(agentNumber),
		endpoint:    strings.TrimSpace(endpoint),
		client:      &http.Client{Timeout: 15 * time.Second},
	}
}

func (t *TataClient) IsSet() bool {
	return t.apiKey != "" && t.apiToken != "" && t.callerID != "" && t.agentNumber != ""
}

func (t *TataClient) InitiateCall(ctx context.Context, toPhone, callbackURL, streamURL string) (string, error) {
	if !t.IsSet() {
		return "", fmt.Errorf("tata: Click-to-Call Support API key, Smartflo API token, caller id and agent number are required")
	}
	payload := map[string]any{
		"customer_number":       TataSupportPhone(toPhone),
		"customer_ring_timeout": 30,
		"api_key":               t.apiKey,
		"caller_id":             strings.TrimPrefix(NormalizePhone(t.callerID), "+"),
		"agent_number":          TataAgentNumber(t.agentNumber),
		"async":                 1,
	}
	if callbackURL != "" {
		payload["custom_identifier"] = callbackURL
		payload["status_callback_url"] = callbackURL
	}
	if streamURL != "" {
		payload["stream_url"] = streamURL
		payload["voice_stream_url"] = streamURL
		payload["media_stream_url"] = streamURL
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("tata: encode request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.endpoint, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("tata: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	// Smartflo requires the account token in the header in addition to the
	// distinct Click-to-Call Support key in the body (since September 2026).
	token := t.apiToken
	if len(token) >= 7 && strings.EqualFold(token[:7], "Bearer ") {
		token = strings.TrimSpace(token[7:])
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := t.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("tata: dial: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("tata: status %d: %s", resp.StatusCode, string(respBody))
	}
	for _, key := range []string{"ref_id", "refId", "call_id", "callId", "callid", "id", "uuid", "request_id", "requestId"} {
		if sid := extractJSON(string(respBody), key); sid != "" {
			return sid, nil
		}
	}
	return "", fmt.Errorf("tata: no call id in response: %s", string(respBody))
}

func (t *TataClient) Hangup(ctx context.Context, callSid string) error {
	callSid = strings.TrimSpace(callSid)
	if callSid == "" {
		return fmt.Errorf("tata: hangup: missing ref id")
	}
	if t.apiToken == "" {
		return fmt.Errorf("tata: hangup: missing api token")
	}

	body, err := json.Marshal(map[string]string{"ref_id": callSid})
	if err != nil {
		return fmt.Errorf("tata: hangup: encode request: %w", err)
	}

	// Smartflo documents Authorization as an access-token header. Accounts in
	// this application historically stored either a raw token or a Bearer value,
	// so try the standard Bearer form first and retry once with the raw value only
	// when authentication is rejected.
	token := strings.TrimSpace(t.apiToken)
	authValues := []string{token}
	if !strings.HasPrefix(strings.ToLower(token), "bearer ") {
		authValues = []string{"Bearer " + token, token}
	}
	var lastStatus int
	var lastBody string
	for idx, authorization := range authValues {
		req, reqErr := http.NewRequestWithContext(ctx, http.MethodPost, t.hangupEndpoint(), bytes.NewReader(body))
		if reqErr != nil {
			return fmt.Errorf("tata: hangup: build request: %w", reqErr)
		}
		req.Header.Set("Authorization", authorization)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")

		resp, doErr := t.client.Do(req)
		if doErr != nil {
			return fmt.Errorf("tata: hangup: http: %w", doErr)
		}
		respBody, readErr := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
		_ = resp.Body.Close()
		if readErr != nil {
			return fmt.Errorf("tata: hangup: read response: %w", readErr)
		}
		lastStatus = resp.StatusCode
		lastBody = strings.TrimSpace(string(respBody))
		if (resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden) && idx+1 < len(authValues) {
			continue
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return fmt.Errorf("tata: hangup: status %d: %s", resp.StatusCode, lastBody)
		}
		var result struct {
			Success *bool  `json:"success"`
			Message string `json:"message"`
		}
		if len(respBody) > 0 && json.Unmarshal(respBody, &result) == nil && result.Success != nil && !*result.Success {
			return fmt.Errorf("tata: hangup rejected: %s", result.Message)
		}
		return nil
	}
	return fmt.Errorf("tata: hangup: status %d: %s", lastStatus, lastBody)
}

func (t *TataClient) hangupEndpoint() string {
	u, err := url.Parse(strings.TrimSpace(t.endpoint))
	if err != nil || u.Scheme == "" || u.Host == "" {
		return defaultTataHangupEndpoint
	}
	u.Path = "/v1/call/hangup"
	u.RawPath = ""
	u.RawQuery = ""
	u.Fragment = ""
	return u.String()
}

func TataSupportPhone(phone string) string {
	phone = strings.TrimPrefix(NormalizePhone(phone), "+")
	if strings.HasPrefix(phone, "91") && len(phone) == 12 {
		return phone[2:]
	}
	return phone
}

func TataAgentNumber(agent string) string {
	agent = strings.Map(func(r rune) rune {
		if r == ' ' || r == '-' || r == '(' || r == ')' || r == '.' || r == '+' {
			return -1
		}
		return r
	}, agent)
	return strings.TrimSpace(agent)
}
