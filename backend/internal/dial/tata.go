package dial

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const defaultTataClickToCallEndpoint = "https://api-smartflo.tatateleservices.com/v1/click_to_call_support"

// TataClient calls Tata Tele Smartflo/CloudPhone APIs.
//
// The first supported path is Smartflo Click-to-Call. Real-time AI calls also
// need Tata VOICE Streaming or SIP enabled for the account; once Tata provides
// the exact stream payloads, the media handler can be mapped separately.
type TataClient struct {
	apiToken    string
	callerID    string
	agentNumber string
	endpoint    string
	client      *http.Client
}

func NewTataClient(apiToken, callerID, agentNumber, endpoint string) *TataClient {
	if endpoint == "" {
		endpoint = defaultTataClickToCallEndpoint
	}
	return &TataClient{
		apiToken:    strings.TrimSpace(apiToken),
		callerID:    strings.TrimSpace(callerID),
		agentNumber: strings.TrimSpace(agentNumber),
		endpoint:    strings.TrimSpace(endpoint),
		client:      &http.Client{Timeout: 15 * time.Second},
	}
}

func (t *TataClient) IsSet() bool {
	return t.apiToken != "" && t.callerID != ""
}

func (t *TataClient) InitiateCall(ctx context.Context, toPhone, callbackURL string) (string, error) {
	if !t.IsSet() {
		return "", fmt.Errorf("tata: api token, caller id and agent number are required")
	}
	payload := map[string]any{
		"customer_number":       TataSupportPhone(toPhone),
		"customer_ring_timeout": 30,
		"api_key":               t.apiToken,
		"caller_id":             strings.TrimPrefix(NormalizePhone(t.callerID), "+"),
		"async":                 1,
	}
	if callbackURL != "" {
		payload["custom_identifier"] = callbackURL
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
	if strings.Contains(t.endpoint, "/click_to_call") && !strings.Contains(t.endpoint, "/click_to_call_support") {
		req.Header.Set("Authorization", "Bearer "+strings.TrimPrefix(t.apiToken, "Bearer "))
	}

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
	return fmt.Errorf("tata: hangup is not implemented until Tata call control API details are provided for call %s", callSid)
}

func TataSupportPhone(phone string) string {
	phone = strings.TrimPrefix(NormalizePhone(phone), "+")
	if strings.HasPrefix(phone, "91") && len(phone) == 12 {
		return phone[2:]
	}
	return phone
}
