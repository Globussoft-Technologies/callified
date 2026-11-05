package realtime

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"time"

	"github.com/gorilla/websocket"
)

type hushReply struct {
	kind int
	pcm  []byte
	err  error
}

// hushInput is owned by the existing Live writer loop, once per call. Keep the
// original bytes until each response is validated so failure cannot mute or
// lose a partial customer frame. No VAD or local interruption logic lives here.
type hushInput struct {
	conn       *websocket.Conn
	replies    chan hushReply
	done       chan struct{}
	frameBytes int
	pending    []byte
}

func newHushInput(ctx context.Context, endpoint string) (*hushInput, error) {
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "ws" && parsed.Scheme != "wss") {
		return nil, fmt.Errorf("invalid Hush WebSocket URL")
	}
	dialer := websocket.Dialer{HandshakeTimeout: 2 * time.Second}
	conn, resp, err := dialer.DialContext(ctx, endpoint, nil)
	if resp != nil && resp.Body != nil {
		resp.Body.Close()
	}
	if err != nil {
		return nil, fmt.Errorf("connect Hush: %w", err)
	}
	conn.SetReadLimit(128 << 10)
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	var ready struct {
		Ready      bool `json:"ready"`
		Rate       int  `json:"sample_rate"`
		FrameBytes int  `json:"frame_bytes"`
	}
	if err = conn.ReadJSON(&ready); err != nil || !ready.Ready || ready.Rate != 8000 || ready.FrameBytes < 2 || ready.FrameBytes > 3200 || ready.FrameBytes%2 != 0 {
		conn.Close()
		return nil, fmt.Errorf("Hush returned invalid readiness/format: %v", err)
	}
	_ = conn.SetReadDeadline(time.Time{})
	p := &hushInput{conn: conn, frameBytes: ready.FrameBytes, replies: make(chan hushReply, 1), done: make(chan struct{})}
	// Continuous reading answers ping/pong during ringing and idle periods.
	go func() {
		for {
			kind, pcm, err := conn.ReadMessage()
			select {
			case p.replies <- hushReply{kind: kind, pcm: pcm, err: err}:
			case <-p.done:
				return
			}
			if err != nil {
				return
			}
		}
	}()
	return p, nil
}
func (p *hushInput) close() {
	if p.conn != nil {
		close(p.done)
		p.conn.Close()
		p.conn = nil
	}
}
func (p *hushInput) process(pcm []byte) []byte {
	if p.conn == nil {
		return pcm
	}
	output := make([]byte, 0, len(pcm)+p.frameBytes)
	for len(pcm) > 0 {
		count := min(len(pcm), 8192)
		chunk := pcm[:count]
		p.pending = append(p.pending, chunk...)
		_ = p.conn.SetWriteDeadline(time.Now().Add(500 * time.Millisecond))
		err := p.conn.WriteMessage(websocket.BinaryMessage, chunk)
		var reply hushReply
		if err == nil {
			timer := time.NewTimer(500 * time.Millisecond)
			select {
			case reply = <-p.replies:
				err = reply.err
			case <-timer.C:
				err = fmt.Errorf("Hush inference timeout")
			}
			timer.Stop()
		}
		expected := len(p.pending) / p.frameBytes * p.frameBytes
		if err == nil && (reply.kind != websocket.BinaryMessage || len(reply.pcm) != expected) {
			err = fmt.Errorf("Hush output length/format mismatch")
		}
		if err != nil {
			p.close()
			slog.Warn("Hush unavailable mid-call; preserving original customer audio", "error", err)
			output = append(output, p.pending...)
			output = append(output, pcm[count:]...)
			p.pending = nil
			return output
		}
		output = append(output, reply.pcm...)
		p.pending = append(p.pending[:0], p.pending[expected:]...)
		pcm = pcm[count:]
	}
	return output
}
