package tts

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

const smallestURL = "https://waves-api.smallest.ai/api/v1/lightning/get_speech"

// SmallestProvider streams TTS via SmallestAI Lightning HTTP endpoint.
// Output is already PCM at 8kHz — no resampling needed.
type SmallestProvider struct{ apiKey string }

func NewSmallest(apiKey string) *SmallestProvider { return &SmallestProvider{apiKey: apiKey} }

func (p *SmallestProvider) Synthesize(ctx context.Context, text, language, voiceID string, onChunk func([]byte)) error {
	if voiceID == "" {
		voiceID = "emily"
	}
	if language == "" {
		language = "en"
	}
	body, _ := json.Marshal(map[string]interface{}{
		"text":           text,
		"voice_id":       voiceID,
		"language":       language,
		"sample_rate":    8000,
		"add_wav_header": false,
		"speed":          1.0,
	})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, smallestURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+p.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("smallest: HTTP %d — %s", resp.StatusCode, string(b))
	}

	// Emit only even-length chunks so 16-bit PCM samples never split across
	// chunk boundaries. An odd trailing byte is carried over to the next read.
	buf := make([]byte, 1024)
	var carry byte
	var haveCarry bool
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			size := n
			if haveCarry {
				size++
			}
			chunk := make([]byte, size)
			if haveCarry {
				chunk[0] = carry
				copy(chunk[1:], buf[:n])
				haveCarry = false
			} else {
				copy(chunk, buf[:n])
			}
			if len(chunk)%2 != 0 {
				carry = chunk[len(chunk)-1]
				chunk = chunk[:len(chunk)-1]
				haveCarry = true
			}
			if len(chunk) > 0 {
				onChunk(chunk)
			}
		}
		if err == io.EOF {
			// Drop the final lone byte if any — safer than padding with zeros.
			return nil
		}
		if err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
	}
}
