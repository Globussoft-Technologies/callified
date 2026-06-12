package wa

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	_ "golang.org/x/image/webp" // register webp decoder
)

var imageDecoder = image.Decode

// ChannelConfig holds the credentials needed to send via a provider.
type ChannelConfig struct {
	OrgID            int64
	Provider         string
	PhoneNumber      string
	APIKey           string
	AppID            string // Gupshup source phone / Wati API URL prefix
	GraphVersion     string // Meta only; defaults to v18.0 when empty
	DefaultProductID int64  // product whose prompt the AI agent uses; 0 = fallback generic
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
	to := strings.TrimPrefix(toPhone, "+")
	body, _ := json.Marshal(map[string]any{
		"messaging_product": "whatsapp",
		"to":                to,
		"type":              "text",
		"text":              map[string]string{"body": text},
	})
	version := cfg.GraphVersion
	if version == "" {
		version = "v18.0"
	}
	u := fmt.Sprintf("https://graph.facebook.com/%s/%s/messages", version, cfg.PhoneNumber)
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
	return doRequest(req)
}

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

// SendImage sends an image message via the provider configured in cfg.
// Currently only Meta (WhatsApp Cloud API) is supported.
func SendImage(ctx context.Context, cfg ChannelConfig, toPhone, imageURL string) error {
	switch cfg.Provider {
	case "meta":
		return sendMetaImage(ctx, cfg, toPhone, imageURL)
	default:
		return fmt.Errorf("image sending not supported for provider: %s", cfg.Provider)
	}
}

func sendMetaImage(ctx context.Context, cfg ChannelConfig, toPhone, imageURL string) error {
	version := cfg.GraphVersion
	if version == "" {
		version = "v18.0"
	}

	// Step 1: download the image locally.
	imgResp, err := http.Get(imageURL)
	if err != nil {
		return fmt.Errorf("download image: %w", err)
	}
	defer imgResp.Body.Close()
	imgBytes, err := io.ReadAll(io.LimitReader(imgResp.Body, 10<<20)) // 10 MB max
	if err != nil {
		return fmt.Errorf("read image: %w", err)
	}
	contentType := imgResp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "image/jpeg"
	}
	// Convert webp → JPEG: Meta silently drops webp even when API returns success.
	if strings.Contains(contentType, "webp") {
		img, _, decErr := imageDecoder(bytes.NewReader(imgBytes))
		if decErr == nil {
			var jpegBuf bytes.Buffer
			if encErr := jpeg.Encode(&jpegBuf, img, &jpeg.Options{Quality: 85}); encErr == nil {
				imgBytes = jpegBuf.Bytes()
				contentType = "image/jpeg"
			}
		}
	}

	// Convert avif → JPEG using Python3/Pillow if available on the server.
	// Go has no native avif decoder; Meta does not reliably deliver avif files.
	if strings.Contains(contentType, "avif") || strings.Contains(imageURL, ".avif") {
		if converted, err := convertAvifToJPEG(imgBytes); err == nil {
			imgBytes = converted
			contentType = "image/jpeg"
		}
		// If conversion fails, continue — upload the avif and let Meta attempt delivery.
	}

	// Step 2: upload to Meta media endpoint to get a media_id.
	var buf bytes.Buffer
	mw := multipartWriter(&buf)
	mw.WriteField("messaging_product", "whatsapp")
	part, _ := mw.CreateFormFile("file", "image.jpg")
	part.Write(imgBytes)
	mw.Close()

	uploadURL := fmt.Sprintf("https://graph.facebook.com/%s/%s/media", version, cfg.PhoneNumber)
	uploadReq, err := http.NewRequestWithContext(ctx, http.MethodPost, uploadURL, &buf)
	if err != nil {
		return err
	}
	uploadReq.Header.Set("Content-Type", mw.FormDataContentType())
	uploadReq.Header.Set("Authorization", "Bearer "+cfg.APIKey)

	uploadResp, err := http.DefaultClient.Do(uploadReq)
	if err != nil {
		return fmt.Errorf("upload image: %w", err)
	}
	defer uploadResp.Body.Close()
	uploadBody, _ := io.ReadAll(uploadResp.Body)
	var uploadResult struct {
		ID string `json:"id"`
	}
	json.Unmarshal(uploadBody, &uploadResult)
	if uploadResult.ID == "" {
		return fmt.Errorf("media upload failed: %s", string(uploadBody))
	}

	// Step 3: send using the media_id (not a link).
	to := strings.TrimPrefix(toPhone, "+")
	sendBody, _ := json.Marshal(map[string]any{
		"messaging_product": "whatsapp",
		"to":                to,
		"type":              "image",
		"image":             map[string]string{"id": uploadResult.ID},
	})
	u := fmt.Sprintf("https://graph.facebook.com/%s/%s/messages", version, cfg.PhoneNumber)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(sendBody))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	return doRequest(req)
}

// multipartWriter wraps mime/multipart to avoid an extra import in the package.
func multipartWriter(buf *bytes.Buffer) *multipart {
	return &multipart{buf: buf, boundary: "waboundary12345"}
}

type multipart struct {
	buf      *bytes.Buffer
	boundary string
}

func (m *multipart) FormDataContentType() string {
	return "multipart/form-data; boundary=" + m.boundary
}

func (m *multipart) WriteField(name, value string) {
	fmt.Fprintf(m.buf, "--%s\r\nContent-Disposition: form-data; name=%q\r\n\r\n%s\r\n", m.boundary, name, value)
}

func (m *multipart) CreateFormFile(fieldname, filename string) (io.Writer, error) {
	fmt.Fprintf(m.buf, "--%s\r\nContent-Disposition: form-data; name=%q; filename=%q\r\nContent-Type: image/jpeg\r\n\r\n", m.boundary, fieldname, filename)
	return m.buf, nil
}

func (m *multipart) Close() {
	fmt.Fprintf(m.buf, "\r\n--%s--\r\n", m.boundary)
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

// convertAvifToJPEG converts an avif image to JPEG using Python3 + Pillow.
// Returns error if python3 or Pillow is not available on this system.
func convertAvifToJPEG(avifBytes []byte) ([]byte, error) {
	tmpIn, err := os.CreateTemp("", "img_*.avif")
	if err != nil {
		return nil, err
	}
	defer os.Remove(tmpIn.Name())
	if _, err := tmpIn.Write(avifBytes); err != nil {
		tmpIn.Close()
		return nil, err
	}
	tmpIn.Close()

	outPath := tmpIn.Name() + ".jpg"
	defer os.Remove(outPath)

	script := fmt.Sprintf(
		`from PIL import Image; img=Image.open(%q); img.convert('RGB').save(%q,'JPEG',quality=85)`,
		tmpIn.Name(), outPath)
	cmd := exec.Command("python3", "-c", script)
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("avif convert: %w — %s", err, string(out))
	}
	return os.ReadFile(outPath)
}

func escapeJSON(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	s = strings.ReplaceAll(s, "\n", `\n`)
	return s
}
