package wshandler

import (
	"context"
	"fmt"
	"time"
)

// ActiveSession is a JSON-friendly snapshot of one in-flight call. Returned
// by Handler.ActiveSessions for the GET /api/active-calls debug endpoint so
// operators can grab a live stream_sid without tailing logs.
type ActiveSession struct {
	StreamSid  string `json:"stream_sid"`
	CallSid    string `json:"call_sid,omitempty"`
	LeadName   string `json:"lead_name,omitempty"`
	LeadPhone  string `json:"lead_phone,omitempty"`
	CampaignID int64  `json:"campaign_id,omitempty"`
	OrgID      int64  `json:"org_id,omitempty"`
	UserID     int64  `json:"user_id,omitempty"`
	UserEmail  string `json:"user_email,omitempty"`
	IsExotel   bool   `json:"is_exotel"`
	IsWebSim   bool   `json:"is_web_sim"`
	StartedAt  string `json:"started_at"` // RFC3339
	DurationS  int    `json:"duration_s"`
	MonitorURL string `json:"monitor_url"`
}

// ActiveSessions returns a snapshot of every live call session. Safe to call
// concurrently — the underlying sync.Map handles iteration. We deduplicate
// by stream_sid because the same session may appear in both the stream and
// call_sid indexes.
func (h *Handler) ActiveSessions() []ActiveSession {
	now := time.Now()
	seen := make(map[string]struct{})
	out := make([]ActiveSession, 0)

	h.sessions.Range(func(_, v any) bool {
		sess, ok := v.(*CallSession)
		if !ok || sess == nil {
			return true
		}
		if _, dup := seen[sess.StreamSid]; dup {
			return true
		}
		seen[sess.StreamSid] = struct{}{}
		out = append(out, ActiveSession{
			StreamSid:  sess.StreamSid,
			CallSid:    sess.CallSid,
			LeadName:   sess.LeadName,
			LeadPhone:  sess.LeadPhone,
			CampaignID: sess.CampaignID,
			OrgID:      sess.OrgID,
			UserID:     sess.UserID,
			UserEmail:  sess.UserEmail,
			IsExotel:   sess.IsExotel,
			IsWebSim:   sess.IsWebSim,
			StartedAt:  sess.CallStart.UTC().Format(time.RFC3339),
			DurationS:  int(now.Sub(sess.CallStart).Seconds()),
			MonitorURL: fmt.Sprintf("/ws/monitor/%s", sess.StreamSid),
		})
		return true
	})
	return out
}

// CloseCall asks the active media WebSocket for a provider call to close and
// schedules post-call persistence. Carrier webhooks can arrive before the
// socket exits naturally; this keeps transcript/recording finalization from
// being missed.
func (h *Handler) CloseCall(callSid string) bool {
	if callSid == "" {
		return false
	}
	raw, ok := h.sessionsByCallSid.Load(callSid)
	if !ok {
		return false
	}
	sess, ok := raw.(*CallSession)
	if !ok || sess == nil {
		return false
	}
	sess.RequestHangup()
	if sess.WS != nil {
		_ = sess.WS.Close()
	}
	go func() {
		time.Sleep(2 * time.Second)
		h.finalizeCall(context.Background(), sess)
	}()
	return true
}
