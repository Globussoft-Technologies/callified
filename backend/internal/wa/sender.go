package wa

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
)

// ChannelConfig holds the credentials needed to send via a provider.
type ChannelConfig struct {
	OrgID       int64
	Provider    string
	PhoneNumber string
	APIKey      string
	AppID       string // Gupshup source phone / Wati API URL prefix
}

var httpClient = &http.Client{Timeout: 15 * time.Second}

// SendText sends a plain text message via the provider configured in cfg.
func SendText(ctx context.Context, cfg ChannelConfig, toPhone, text string) error {
	switch cfg.Provider {
	case "gupshup":
		return sendGupshupText(ctx, cfg, toPhone, text)
	case "wati":
		return sendWatiText(ctx, cfg, toPhone, text)
	case "interakt":
		return sendInteraktText(ctx, cfg, toPhone, text)
	case "meta":
		return sendMetaText(ctx, cfg, toPhone, text)
	case "aisensei":
		return sendGupshupText(ctx, cfg, toPhone, text) // same API
	case "wasender":
		return sendWaSenderText(ctx, cfg, toPhone, text)
	default:
		return fmt.Errorf("unknown WA provider: %s", cfg.Provider)
	}
}

func sendGupshupText(ctx context.Context, cfg ChannelConfig, toPhone, text string) error {
	payload := map[string]string{
		"channel":  "whatsapp",
		"source":   cfg.PhoneNumber,
		"destination": toPhone,
		"message":  fmt.Sprintf(`{"type":"text","text":"%s"}`, escapeJSON(text)),
		"src.name": cfg.AppID,
	}
	return doFormPost(ctx,
		"https://api.gupshup.io/sm/api/v1/msg",
		map[string]string{"apikey": cfg.APIKey},
		payload)
}

func sendWatiText(ctx context.Context, cfg ChannelConfig, toPhone, text string) error {
	// Wati REST: POST {apiURL}/api/v1/sendSessionMessage/{phone}
	apiURL := cfg.AppID
	if apiURL == "" {
		apiURL = "https://live-mt-server.wati.io"
	}
	u := fmt.Sprintf("%s/api/v1/sendSessionMessage/%s", strings.TrimRight(apiURL, "/"), toPhone)
	body, _ := json.Marshal(map[string]string{"messageText": text})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	return doRequest(req)
}

func sendInteraktText(ctx context.Context, cfg ChannelConfig, toPhone, text string) error {
	body, _ := json.Marshal(map[string]any{
		"countryCode": "+91",
		"phoneNumber": toPhone,
		"type":        "text",
		"data":        map[string]string{"message": text},
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://api.interakt.ai/v1/public/message/", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Basic "+cfg.APIKey)
	return doRequest(req)
}

func sendMetaText(ctx context.Context, cfg ChannelConfig, toPhone, text string) error {
	body, _ := json.Marshal(map[string]any{
		"messaging_product": "whatsapp",
		"to":                toPhone,
		"type":              "text",
		"text":              map[string]string{"body": text},
	})
	u := fmt.Sprintf("https://graph.facebook.com/v18.0/%s/messages", cfg.PhoneNumber)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	return doRequest(req)
}

func sendWaSenderText(ctx context.Context, cfg ChannelConfig, toPhone, text string) error {
	// Default to the production WaSender API host. cfg.AppID can override
	// for self-hosted deployments. Marketing site (www.wasender.app) is
	// NOT the API — it's www.wasenderapi.com per their public docs.
	baseURL := cfg.AppID
	if baseURL == "" {
		baseURL = "https://www.wasenderapi.com"
	}
	baseURL = strings.TrimRight(baseURL, "/")

	// WaSender uses two different tokens:
	//   - PAT (Personal Access Token, e.g. "5183|H9k7n…") for managing
	//     sessions (list, connect, QR).
	//   - Session API key (per-session, hex64) for /api/send-message.
	// Users save their PAT in the modal because that's what unlocks the
	// QR-scan flow. We resolve the session key here at send time so the
	// dashboard never has to ask the user for two separate tokens.
	sendKey, err := resolveWaSenderSessionKey(ctx, baseURL, cfg.APIKey)
	if err != nil {
		return fmt.Errorf("resolve session key: %w", err)
	}

	// WaSender expects the recipient in E.164 with the leading +.
	to := strings.TrimSpace(toPhone)
	if to != "" && !strings.HasPrefix(to, "+") {
		to = "+" + to
	}
	// Guard: never self-send. Replying to our own device number happens
	// when an inbound payload is malformed (FromPhone == device number)
	// or a dashboard test seeds a conversation with the device's own
	// phone. WaSender returns 200 with success:false for these and we'd
	// otherwise persist a phantom outbound bubble. Cheap to detect here.
	devicePhone := strings.TrimSpace(cfg.PhoneNumber)
	if devicePhone != "" && !strings.HasPrefix(devicePhone, "+") {
		devicePhone = "+" + devicePhone
	}
	if to == devicePhone {
		return fmt.Errorf("refusing to send to our own device number %s", to)
	}

	body, _ := json.Marshal(map[string]any{
		"to":   to,
		"text": text,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		baseURL+"/api/send-message", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+sendKey)
	// Use doRequestVerbose so the WaSender response body is captured in
	// the returned error on non-2xx — and the success body is surfaced via
	// the diagnostic "wasender send" log line below. Without this we had
	// no way to tell whether a 200 from WaSender actually delivered (their
	// API returns 200 even when the JID is unreachable, with the failure
	// detail inside the JSON body).
	respBody, err := doRequestVerbose(req)
	if err != nil {
		return err
	}
	if waSenderLogger != nil {
		waSenderLogger.Sugar().Infow("wasender send", "to", to, "body", string(respBody))
	}
	// WaSender returns HTTP 200 with `{"success":false,"message":"…"}`
	// for soft failures (session disconnected, invalid JID, rate limit).
	// Treat these as send errors so the dashboard doesn't persist a
	// "sent" bubble for a message that never left WaSender.
	var parsed struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	if jsonErr := json.Unmarshal(respBody, &parsed); jsonErr == nil && !parsed.Success {
		msg := parsed.Message
		if msg == "" {
			msg = "WaSender returned success=false"
		}
		return fmt.Errorf("WaSender: %s", msg)
	}
	return nil
}

// waSenderLogger is wired from main.go via SetWaSenderLogger so the package
// can log without taking a logger on every call. nil = no-op.
var waSenderLogger *zap.Logger

// SetWaSenderLogger lets the bootstrap install a zap logger for WaSender
// diagnostics. Optional — sender works without it.
func SetWaSenderLogger(l *zap.Logger) { waSenderLogger = l }

// resolveWaSenderSessionKey looks up the first connected session's
// per-session api_key by hitting /api/whatsapp-sessions with the saved
// PAT. We prefer a session whose status is "connected" so a stale
// half-logged-out session doesn't shadow a working one. Cached for 60s
// to avoid one round-trip per outbound message.
//
// If the saved token is already a session key (hex64, no `|` separator),
// we use it directly — older configs predate the dual-token UX and may
// still hold the per-session key.
type waSenderKeyCacheEntry struct {
	key     string
	expires time.Time
}

var (
	waSenderKeyCacheMu sync.Mutex
	waSenderKeyCache   = map[string]waSenderKeyCacheEntry{}
)

func resolveWaSenderSessionKey(ctx context.Context, baseURL, savedToken string) (string, error) {
	if savedToken == "" {
		return "", fmt.Errorf("no token configured")
	}
	// Heuristic: PATs look like "<digits>|<random>" (a Laravel Sanctum
	// shape WaSender uses). Session keys are 64 hex chars. If it doesn't
	// contain a |, treat it as a session key and skip the lookup.
	if !strings.Contains(savedToken, "|") {
		return savedToken, nil
	}

	waSenderKeyCacheMu.Lock()
	if e, ok := waSenderKeyCache[savedToken]; ok && time.Now().Before(e.expires) {
		waSenderKeyCacheMu.Unlock()
		return e.key, nil
	}
	waSenderKeyCacheMu.Unlock()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/api/whatsapp-sessions", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+savedToken)
	req.Header.Set("Accept", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		buf, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return "", fmt.Errorf("list sessions HTTP %d: %s", resp.StatusCode, string(buf))
	}
	var parsed struct {
		Success bool `json:"success"`
		Data    []struct {
			ID     int64  `json:"id"`
			Status string `json:"status"`
			APIKey string `json:"api_key"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return "", fmt.Errorf("decode sessions: %w", err)
	}
	// Prefer connected sessions; fall back to first one if none are
	// connected (allows the call to attempt a send anyway, with a clear
	// error from WaSender if the session isn't ready).
	pick := ""
	for _, s := range parsed.Data {
		if strings.EqualFold(s.Status, "connected") && s.APIKey != "" {
			pick = s.APIKey
			break
		}
	}
	if pick == "" && len(parsed.Data) > 0 {
		pick = parsed.Data[0].APIKey
	}
	if pick == "" {
		return "", fmt.Errorf("no whatsapp sessions found for this PAT")
	}

	waSenderKeyCacheMu.Lock()
	waSenderKeyCache[savedToken] = waSenderKeyCacheEntry{key: pick, expires: time.Now().Add(60 * time.Second)}
	waSenderKeyCacheMu.Unlock()
	return pick, nil
}

func doFormPost(ctx context.Context, url string, headers, fields map[string]string) error {
	var buf bytes.Buffer
	first := true
	for k, v := range fields {
		if !first {
			buf.WriteByte('&')
		}
		buf.WriteString(k + "=" + v)
		first = false
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, &buf)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	return doRequest(req)
}

func doRequest(req *http.Request) error {
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("WA send: HTTP %d — %s", resp.StatusCode, string(body))
	}
	return nil
}

// doRequestVerbose is doRequest that also returns the response body on
// 2xx so callers can log delivery acknowledgements (and diagnose providers
// like WaSender that return 200 even on unrouteable JIDs).
func doRequestVerbose(req *http.Request) ([]byte, error) {
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<14))
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("WA send: HTTP %d — %s", resp.StatusCode, string(body))
	}
	return body, nil
}

func escapeJSON(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	s = strings.ReplaceAll(s, "\n", `\n`)
	return s
}
