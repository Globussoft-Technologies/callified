package dial

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type PlivoClient struct {
	AuthID, AuthToken string
	HTTP              *http.Client
	baseURL           string
}

func NewPlivoClient(authID, token string) *PlivoClient {
	return &PlivoClient{AuthID: strings.TrimSpace(authID), AuthToken: strings.TrimSpace(token), HTTP: &http.Client{Timeout: 15 * time.Second}, baseURL: "https://api.plivo.com/v1/Account/"}
}

func (p *PlivoClient) request(ctx context.Context, method, path string, body any) ([]byte, error) {
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, method, p.baseURL+url.PathEscape(p.AuthID)+"/"+path, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(p.AuthID, p.AuthToken)
	req.Header.Set("Content-Type", "application/json")
	res, err := p.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("plivo request: %w", err)
	}
	defer res.Body.Close()
	data, err := io.ReadAll(io.LimitReader(res.Body, 65536))
	if err != nil {
		return nil, err
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, fmt.Errorf("plivo returned HTTP %d", res.StatusCode)
	}
	return data, nil
}

func (p *PlivoClient) InitiateCall(ctx context.Context, from, to, answerURL, statusURL string) (string, error) {
	raw, err := p.request(ctx, http.MethodPost, "Call/", map[string]any{"from": NormalizePhone(from), "to": NormalizePhone(to), "answer_url": answerURL, "answer_method": "POST", "ring_url": statusURL, "ring_method": "POST", "hangup_url": statusURL, "hangup_method": "POST"})
	if err != nil {
		return "", err
	}
	var result struct {
		RequestUUID string `json:"request_uuid"`
	}
	if err = json.Unmarshal(raw, &result); err != nil {
		return "", err
	}
	if result.RequestUUID == "" {
		return "", fmt.Errorf("plivo response missing request_uuid")
	}
	return result.RequestUUID, nil
}

func (p *PlivoClient) Hangup(ctx context.Context, sid string) error {
	if strings.TrimSpace(sid) == "" {
		return fmt.Errorf("plivo call UUID required")
	}
	_, err := p.request(ctx, http.MethodDelete, "Call/"+url.PathEscape(sid)+"/", nil)
	return err
}

func (p *PlivoClient) CancelRequest(ctx context.Context, id string) error {
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("plivo request UUID required")
	}
	_, err := p.request(ctx, http.MethodDelete, "Request/"+url.PathEscape(id)+"/", nil)
	return err
}
