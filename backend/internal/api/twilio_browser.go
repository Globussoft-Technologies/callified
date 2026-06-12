package api

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// ── GET /api/campaigns/{id}/twilio-token ────────────────────────────────────
// Returns a Twilio Access Token (JWT) that the browser Voice SDK uses to make
// outbound calls. The token is scoped to the Twilio account linked to the
// campaign via Provider Accounts.

func (s *Server) twilioToken(w http.ResponseWriter, r *http.Request) {
	campaignID, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid campaign id")
		return
	}

	// Look up the campaign's linked provider account (must be Twilio).
	creds, err := s.db.GetCampaignExotelCreds(campaignID)
	if err != nil || creds.Provider != "twilio" || !creds.IsSet() {
		writeError(w, http.StatusBadRequest, "campaign has no Twilio account configured")
		return
	}

	// creds field mapping for Twilio:
	//   APIKey     = Auth Token  (used as HMAC key? No — see below)
	//   APIToken   = API Key SID (SK…)
	//   APISecret  = API Secret  (signing key)
	//   AccountSID = Account SID (AC…)
	//   CallerID   = From phone
	//   AppID      = TwiML App SID (AP…) — used as outgoing application

	apiKeySID := creds.APIToken  // SK…
	apiSecret := creds.APISecret
	accountSID := creds.AccountSID
	appSID := creds.AppID

	if apiKeySID == "" || apiSecret == "" {
		writeError(w, http.StatusBadRequest, "Twilio API Key SID and API Secret are required for browser calls")
		return
	}

	identity := fmt.Sprintf("agent-%d", campaignID)
	now := time.Now().Unix()

	token, err := buildTwilioAccessToken(accountSID, apiKeySID, apiSecret, appSID, identity, now)
	if err != nil {
		s.logger.Sugar().Errorw("twilioToken: build failed", "err", err)
		writeError(w, http.StatusInternalServerError, "failed to generate token")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"token":    token,
		"identity": identity,
	})
}

// buildTwilioAccessToken builds a signed JWT that the Twilio Voice SDK accepts.
// Format: https://www.twilio.com/docs/iam/access-tokens
func buildTwilioAccessToken(accountSID, apiKeySID, apiSecret, appSID, identity string, now int64) (string, error) {
	header := map[string]string{
		"cty": "twilio-fpa;v=1",
		"typ": "JWT",
		"alg": "HS256",
	}

	grants := map[string]any{
		"identity": identity,
		"voice": map[string]any{
			"incoming": map[string]bool{"allow": true},
		},
	}
	if appSID != "" {
		grants["voice"].(map[string]any)["outgoing"] = map[string]string{
			"application_sid": appSID,
		}
	}

	payload := map[string]any{
		"jti":    fmt.Sprintf("%s-%d", apiKeySID, now),
		"iss":    apiKeySID,
		"sub":    accountSID,
		"exp":    now + 3600,
		"grants": grants,
	}

	headerJSON, err := json.Marshal(header)
	if err != nil {
		return "", err
	}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	headerEnc := base64.RawURLEncoding.EncodeToString(headerJSON)
	payloadEnc := base64.RawURLEncoding.EncodeToString(payloadJSON)
	signingInput := headerEnc + "." + payloadEnc

	mac := hmac.New(sha256.New, []byte(apiSecret))
	mac.Write([]byte(signingInput))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))

	return signingInput + "." + sig, nil
}

// ── POST /webhook/twilio/voice ───────────────────────────────────────────────
// Twilio calls this when the browser SDK places an outbound call via
// device.connect({ params: { To: phone, CallerId: fromPhone } }).
// We return TwiML that dials the customer's phone number directly — no media
// stream relay, pure WebRTC ↔ PSTN bridge by Twilio (zero server delay).

func (s *Server) twilioVoiceWebhook(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		writeError(w, http.StatusBadRequest, "invalid form")
		return
	}

	to := strings.TrimSpace(r.FormValue("To"))
	callerID := strings.TrimSpace(r.FormValue("CallerId"))

	// Normalize: Twilio requires E.164 format (+91XXXXXXXXXX).
	to = normalizeE164(to)

	if to == "" {
		w.Header().Set("Content-Type", "text/xml")
		fmt.Fprint(w, `<?xml version="1.0" encoding="UTF-8"?><Response><Hangup/></Response>`)
		return
	}

	var callerIDAttr string
	if callerID != "" {
		callerIDAttr = fmt.Sprintf(` callerId="%s"`, callerID)
	}

	twiml := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<Response>
  <Dial%s timeout="30">
    <Number>%s</Number>
  </Dial>
</Response>`, callerIDAttr, to)

	w.Header().Set("Content-Type", "text/xml")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(twiml))
}

// normalizeE164 converts 10-digit Indian numbers to +91XXXXXXXXXX.
func normalizeE164(phone string) string {
	// Strip spaces, dashes, parentheses.
	var digits strings.Builder
	for _, c := range phone {
		if c >= '0' && c <= '9' {
			digits.WriteRune(c)
		} else if c == '+' && digits.Len() == 0 {
			digits.WriteRune(c)
		}
	}
	s := digits.String()
	if strings.HasPrefix(s, "+") {
		return s
	}
	if len(s) == 10 {
		return "+91" + s
	}
	if len(s) == 12 && strings.HasPrefix(s, "91") {
		return "+" + s
	}
	return s
}
