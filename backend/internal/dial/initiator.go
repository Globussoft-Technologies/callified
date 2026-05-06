package dial

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"

	"go.uber.org/zap"

	"github.com/globussoft/callified-backend/internal/callguard"
	"github.com/globussoft/callified-backend/internal/config"
	"github.com/globussoft/callified-backend/internal/db"
	rstore "github.com/globussoft/callified-backend/internal/redis"
	"github.com/globussoft/callified-backend/internal/webhook"
)

// urlQueryEscape is just url.QueryEscape spelled out to keep the long
// fmt.Sprintf legible.
func urlQueryEscape(s string) string { return url.QueryEscape(s) }

// CallData holds the information needed to initiate one outbound call.
type CallData struct {
	LeadID      int64
	LeadName    string
	LeadPhone   string
	CampaignID  int64
	OrgID       int64
	Interest    string
	Language    string
	TTSProvider string
	TTSVoiceID  string
	TTSLanguage string
}

// Initiator orchestrates the full dial sequence:
// DND check → TRAI hours → Redis pending call → provider dial → DB log.
type Initiator struct {
	cfg     *config.Config
	store   *rstore.Store
	db      *db.DB
	disp    *webhook.Dispatcher
	twilio  *TwilioClient
	exotel  *ExotelClient
	log     *zap.Logger
}

// New creates an Initiator wired to both telephony providers.
func New(cfg *config.Config, store *rstore.Store, database *db.DB, disp *webhook.Dispatcher, log *zap.Logger) *Initiator {
	return &Initiator{
		cfg:    cfg,
		store:  store,
		db:     database,
		disp:   disp,
		twilio: NewTwilioClient(cfg.TwilioAccountSID, cfg.TwilioAuthToken, cfg.TwilioPhone),
		exotel: NewExotelClient(cfg.ExotelAPIKey, cfg.ExotelAPIToken, cfg.ExotelAccountSID, cfg.ExotelCallerID, cfg.ExotelAppID),
		log:    log,
	}
}

// ErrDND is returned when the lead is on the DND list.
var ErrDND = fmt.Errorf("lead is on DND list")

// ErrCallHours is returned when TRAI calling hours are not active.
var ErrCallHours = fmt.Errorf("outside TRAI calling hours (9 AM – 9 PM)")

// ErrInsufficientCredits is returned when the org's prepaid balance is zero
// or negative. Surfaced to the API handler so it can return HTTP 402 with a
// "recharge to continue" message instead of letting Exotel be charged for a
// dial we can't bill the customer for.
var ErrInsufficientCredits = fmt.Errorf("insufficient credits — please recharge to continue making calls")

// Initiate performs the full dial sequence for one lead.
// Returns the carrier-issued call SID plus nil on successful dial initiation
// (not call completion). The call_sid lets callers index the call for later
// lookup — e.g., the manual-call REST endpoint returns it so external clients
// can open /ws/monitor/{call_sid} before the media stream connects.
func (i *Initiator) Initiate(ctx context.Context, data CallData) (string, error) {
	// 1. DND check
	isDND, err := i.db.IsDNDNumber(data.OrgID, data.LeadPhone)
	if err != nil {
		i.log.Warn("dial: DND check failed", zap.Error(err))
	}
	if isDND {
		_ = i.db.UpdateLeadStatus(data.LeadID, "DND — do not call")
		// Live-feed: tell the campaign detail page why this number was skipped.
		i.store.EmitCampaignEvent(ctx, data.CampaignID, data.LeadName, data.LeadPhone, "dnd", "number is on DND list")
		return "", ErrDND
	}

	// 2. TRAI calling hours
	tz, _ := i.db.GetOrgTimezone(data.OrgID)
	status := callguard.Check(tz)
	if !status.Allowed {
		return "", fmt.Errorf("%w: %s", ErrCallHours, status.Reason)
	}

	// 2.5 Credit balance gate. Real telephony calls cost money — we won't
	// dial the provider for an org that can't pay for it. Web-sim is free
	// (it doesn't go through this Initiator at all), so the gate only
	// affects outbound Exotel/Twilio dials.
	//
	// OrgID==0 happens in a few legacy/test code paths; let those through
	// so we don't break dev environments with no billing setup.
	if data.OrgID > 0 {
		oc, ocErr := i.db.GetOrgCredit(data.OrgID)
		if ocErr != nil {
			i.log.Warn("dial: GetOrgCredit failed; allowing call", zap.Error(ocErr))
		} else if oc != nil && oc.BalancePaise <= 0 {
			_ = i.db.UpdateLeadStatus(data.LeadID, "Insufficient Credits")
			i.store.EmitCampaignEvent(ctx, data.CampaignID, data.LeadName, data.LeadPhone,
				"failed", "insufficient credits — recharge to continue")
			return "", ErrInsufficientCredits
		}
	}

	// 3. Store pending call info in Redis (wshandler reads this on stream connect)
	pending := rstore.PendingCallInfo{
		Name:        data.LeadName,
		Phone:       data.LeadPhone,
		LeadID:      data.LeadID,
		OrgID:       data.OrgID,
		Interest:    data.Interest,
		CampaignID:  data.CampaignID,
		TTSProvider: data.TTSProvider,
		TTSVoiceID:  data.TTSVoiceID,
		TTSLanguage: data.TTSLanguage,
	}

	// 4. Dial via the configured provider
	provider := i.cfg.DefaultProvider
	var callSid string

	switch provider {
	case "twilio":
		twimlURL := fmt.Sprintf("%s/webhook/twilio?lead_id=%d&campaign_id=%d",
			i.cfg.PublicServerURL, data.LeadID, data.CampaignID)
		statusURL := fmt.Sprintf("%s/webhook/twilio/status", i.cfg.PublicServerURL)
		callSid, err = i.twilio.InitiateCall(ctx, data.LeadPhone, twimlURL, statusURL)
	default: // exotel
		// Pass our own /webhook/exotel as the ExoML URL with the per-call
		// context query-encoded — when Exotel opens the WebSocket it will
		// hit our exotelXML handler with these params and we control the
		// returned <Stream> URL. The earlier "use the dashboard app URL"
		// approach broke because the configured app at appID=1210468
		// just plays voice-404.mp3 ("number not reachable" in Hindi) and
		// hangs up, so calls never reached our webhook at all.
		exomlURL := fmt.Sprintf(
			"%s/webhook/exotel?name=%s&interest=%s&phone=%s&lead_id=%d&campaign_id=%d&org_id=%d&tts_provider=%s&voice=%s&tts_language=%s",
			i.cfg.PublicServerURL,
			urlQueryEscape(data.LeadName),
			urlQueryEscape(data.Interest),
			urlQueryEscape(data.LeadPhone),
			data.LeadID, data.CampaignID, data.OrgID,
			urlQueryEscape(data.TTSProvider),
			urlQueryEscape(data.TTSVoiceID),
			urlQueryEscape(data.TTSLanguage),
		)
		statusURL := fmt.Sprintf("%s/webhook/exotel/status?lead_id=%d&campaign_id=%d",
			i.cfg.PublicServerURL, data.LeadID, data.CampaignID)
		callSid, err = i.exotel.InitiateCall(ctx, data.LeadPhone, exomlURL, statusURL)
	}
	if err != nil {
		_ = i.db.UpdateLeadStatus(data.LeadID, fmt.Sprintf("Call Failed (%s)", provider))
		// Live-feed: surface the dial-time failure (bad params, provider
		// rejected, etc.) on the campaign detail page.
		i.store.EmitCampaignEvent(ctx, data.CampaignID, data.LeadName, data.LeadPhone, "failed", fmt.Sprintf("%s: %v", provider, err))
		return "", fmt.Errorf("dial %s: %w", provider, err)
	}

	// 5. Persist pending call under the call SID for webhook lookup
	pending.ExotelCallSid = callSid
	if storeErr := i.store.SetPendingCall(ctx, callSid, pending); storeErr != nil {
		i.log.Warn("dial: SetPendingCall failed", zap.Error(storeErr))
	}
	// Also store under "latest" for fallback in wshandler
	_ = i.store.SetPendingCall(ctx, "latest", pending)

	// 6. Log dial attempt in DB
	if _, dbErr := i.db.SaveCallLog(data.LeadID, data.CampaignID, data.OrgID,
		callSid, provider, data.LeadPhone, "initiated"); dbErr != nil {
		i.log.Warn("dial: SaveCallLog failed", zap.Error(dbErr))
	}
	_ = i.db.IncrLeadDialAttempts(data.LeadID)
	_ = i.db.UpdateLeadStatus(data.LeadID, "Calling")

	i.log.Info("call initiated",
		zap.String("provider", provider),
		zap.String("call_sid", callSid),
		zap.Int64("lead_id", data.LeadID),
		zap.Int64("campaign_id", data.CampaignID),
	)
	// Live-feed: dial went out successfully.
	i.store.EmitCampaignEvent(ctx, data.CampaignID, data.LeadName, data.LeadPhone, "dialing", fmt.Sprintf("via %s", provider))

	// 7. Fire dial.initiated webhook
	dialData, _ := json.Marshal(map[string]any{
		"call_sid":    callSid,
		"lead_id":     data.LeadID,
		"campaign_id": data.CampaignID,
		"phone":       data.LeadPhone,
		"provider":    provider,
	})
	_ = dialData
	i.disp.Dispatch(ctx, data.OrgID, "call.initiated", map[string]any{
		"call_sid":    callSid,
		"lead_id":     data.LeadID,
		"campaign_id": data.CampaignID,
	})

	return callSid, nil
}
