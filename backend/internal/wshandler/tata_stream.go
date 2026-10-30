package wshandler

import (
	"context"
	"fmt"
	"strings"
	"time"

	rstore "github.com/globussoft/callified-backend/internal/redis"
)

func applyVerifiedTataContext(sess *CallSession, info *rstore.TataStreamContext) {
	p := info.Pending
	sess.OrgID = p.OrgID
	sess.LeadID = p.LeadID
	sess.CampaignID = p.CampaignID
	sess.CallSid = p.ExotelCallSid
	if sess.CallSid == "" {
		sess.CallSid = info.CallID
	}
	sess.LeadName = p.Name
	sess.LeadPhone = p.Phone
	sess.Interest = p.Interest
	sess.UserID = p.UserID
	sess.UserEmail = p.UserEmail
	sess.SkipCredits = p.SkipCredits
	sess.TTSProvider = p.TTSProvider
	sess.TTSVoiceID = p.TTSVoiceID
	sess.TTSLanguage = p.TTSLanguage
	sess.Language = p.TTSLanguage
	sess.MaxCallDurationSeconds = p.MaxCallDurationSeconds
	sess.IsInbound = info.Inbound
}
func validateTataStart(info *rstore.TataStreamContext, event map[string]interface{}) error {
	start, _ := event["start"].(map[string]interface{})
	if start == nil {
		return fmt.Errorf("missing start")
	}
	// callId is the carrier call identity, not the REST request/ref id.
	id := pickStr(start, "callId", "call_id", "callSid", "CallSid", "call_sid")
	if id == "" {
		id = pickStr(event, "callId", "call_id", "callSid", "call_sid")
	}
	if id != info.CallID {
		return fmt.Errorf("call identity mismatch")
	}
	for _, pair := range []struct {
		keys     []string
		expected string
	}{
		{[]string{"from", "fromNumber", "from_number"}, info.From},
		{[]string{"to", "toNumber", "to_number"}, info.To},
	} {
		if v := pickStr(start, pair.keys...); v != "" && rstore.NormalizeTataNumber(v) != rstore.NormalizeTataNumber(pair.expected) {
			return fmt.Errorf("phone identity mismatch")
		}
	}
	return nil
}
func (h *Handler) handleVerifiedTataStart(ctx context.Context, sess *CallSession, event map[string]interface{}) {
	if sess.TataStartVerified {
		return
	}
	if validateTataStart(sess.VerifiedTata, event) != nil {
		sess.WS.Close()
		return
	}
	start, _ := event["start"].(map[string]interface{})
	sid := pickStr(start, "streamSid", "stream_sid", "StreamSid")
	if sid == "" {
		sess.WS.Close()
		return
	}
	sess.StreamSid = sid
	sess.UpdateStreamType()
	sess.UseUlaw = true
	if media, ok := start["mediaFormat"].(map[string]interface{}); ok {
		encoding := strings.ToLower(pickStr(media, "encoding"))
		if encoding != "" && !strings.Contains(encoding, "mulaw") && !strings.Contains(encoding, "ulaw") {
			sess.WS.Close()
			return
		}
	}
	applyVerifiedTataContext(sess, sess.VerifiedTata)
	sess.TataStartVerified = true
	_ = sess.WS.SetReadDeadline(time.Time{})
	h.sessionsByCallSid.Store(sess.CallSid, sess)
	if h.promptBuilder != nil {
		if err := h.initializeCall(ctx, sess); err != nil {
			sess.WS.Close()
			return
		}
		if sess.IsInbound {
			h.applyInboundReceptionistPrompt(sess)
		}
	}
	h.configureVoicePipeline(sess)
	sess.MarkLiveContextReady()
	if sess.StartSTT != nil && sess.Language != "" {
		sess.StartSTT()
		sess.StartSTT = nil
	}
	if sess.SendGreeting != nil {
		sess.SendGreeting()
	}
	h.startMaxCallDurationTimer(ctx, sess)
}
