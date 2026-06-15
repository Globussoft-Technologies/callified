package dial

import (
	"context"
	"encoding/json"
	"fmt"

	"go.uber.org/zap"

	"github.com/globussoft/callified-backend/internal/callguard"
	"github.com/globussoft/callified-backend/internal/config"
	"github.com/globussoft/callified-backend/internal/db"
	rstore "github.com/globussoft/callified-backend/internal/redis"
	"github.com/globussoft/callified-backend/internal/webhook"
)

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
	// IsBridge=true routes the call to browser-to-phone mode: the Exotel stream is
	// relayed to the agent's browser WebSocket instead of the AI pipeline.
	IsBridge bool
	// UserEmail identifies the agent who clicked the call button. Used to honour
	// per-user feature flags such as hide_ai_features → unlimited manual calls.
	UserEmail string
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
		exotel: NewExotelClient(cfg.ExotelAPIKey, cfg.ExotelAPIToken, cfg.ExotelAccountSID, cfg.ExotelCallerID, cfg.ExotelAppID, ""),
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
	//
	// Bypass: agents whose AI features are hidden and who are placing a manual
	// browser call (IsBridge) get unlimited calls — credits are neither checked
	// nor deducted for those calls.
	skipCredits := false
	if data.OrgID > 0 {
		if data.UserEmail != "" && data.IsBridge && i.db.ShouldHideAiFeatures(data.UserEmail) {
			skipCredits = true
			i.log.Info("dial: unlimited manual call for AI-hidden user – skipping credit gate",
				zap.String("email", data.UserEmail),
				zap.Int64("org_id", data.OrgID),
				zap.Int64("lead_id", data.LeadID))
		} else {
			oc, ocErr := i.db.GetOrgCredit(data.OrgID)
			if ocErr != nil {
				i.log.Warn("dial: GetOrgCredit failed; allowing call", zap.Error(ocErr))
			} else if oc != nil && oc.BalancePaise <= 0 {
				// Three passes before blocking:
				// 1. Active subscription → always allow.
				// 2. No deduction history → org is new / never topped up; allow so
				//    fresh orgs and test environments aren't dead-on-arrival.
				// 3. Has prior deductions and balance=0 → genuinely exhausted.
				sub, _ := i.db.GetSubscriptionByOrg(data.OrgID)
				if sub != nil {
					i.log.Info("dial: zero balance but active subscription – allowing call",
						zap.Int64("org_id", data.OrgID), zap.String("plan", sub.PlanName))
				} else {
					hasHistory, _ := i.db.HasCallDeductions(data.OrgID)
					if hasHistory {
						_ = i.db.UpdateLeadStatus(data.LeadID, "Insufficient Credits")
						i.store.EmitCampaignEvent(ctx, data.CampaignID, data.LeadName, data.LeadPhone,
							"failed", "insufficient credits – recharge to continue")
						return "", ErrInsufficientCredits
					}
					i.log.Info("dial: zero balance, no prior deductions – allowing call (new org)",
						zap.Int64("org_id", data.OrgID))
				}
			}
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
		IsBridge:    data.IsBridge,
		SkipCredits: skipCredits,
		UserEmail:   data.UserEmail,
	}

	// 4. Resolve per-campaign provider credentials.
	// Provider is determined by the account linked to the campaign, not by global config.
	var creds db.ExotelCreds
	if data.CampaignID > 0 {
		if c, cerr := i.db.GetCampaignExotelCreds(data.CampaignID); cerr == nil {
			creds = c
		}
	}
	provider := creds.Provider
	if provider == "" {
		provider = i.cfg.DefaultProvider
	}
	// Carry the Exotel app/flow type through to the webhook so it can return
	// the correct response format (XML for legacy ExoML, JSON for AgentStream).
	pending.AppType = creds.AppType
	var callSid string

	switch provider {
	case "twilio":
		var twilioClient *TwilioClient
		if creds.IsSet() {
			// accountSID, authToken (=APIKey), fromPhone (=CallerID)
			twilioClient = NewTwilioClient(creds.AccountSID, creds.APIKey, creds.CallerID)
		} else {
			twilioClient = i.twilio // global fallback
		}
		twimlURL := fmt.Sprintf("%s/webhook/twilio?lead_id=%d&campaign_id=%d",
			i.cfg.PublicServerURL, data.LeadID, data.CampaignID)
		statusURL := fmt.Sprintf("%s/webhook/twilio/status", i.cfg.PublicServerURL)
		callSid, err = twilioClient.InitiateCall(ctx, data.LeadPhone, twimlURL, statusURL)
	default: // exotel
		if !creds.IsSet() {
			i.store.EmitCampaignEvent(ctx, data.CampaignID, data.LeadName, data.LeadPhone, "failed", "no campaign Exotel credentials set")
			return "", fmt.Errorf("no Exotel credentials configured for this campaign")
		}
		exotelClient := NewExotelClient(creds.APIKey, creds.APIToken, creds.AccountSID, creds.CallerID, creds.AppID, creds.AppType)
		statusURL := fmt.Sprintf("%s/webhook/exotel/status?lead_id=%d&campaign_id=%d",
			i.cfg.PublicServerURL, data.LeadID, data.CampaignID)
		callSid, err = exotelClient.InitiateCall(ctx, data.LeadPhone, "", statusURL)
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

// Hangup ends an in-progress carrier call. It resolves the campaign's provider
// credentials (falling back to global config) and issues a provider-side hang-up
// so the remote party's line is released when the agent clicks Hang Up.
func (i *Initiator) Hangup(ctx context.Context, callSid string, campaignID int64) error {
	if callSid == "" {
		return fmt.Errorf("missing call sid")
	}

	var creds db.ExotelCreds
	if campaignID > 0 {
		creds, _ = i.db.GetCampaignExotelCreds(campaignID)
	}

	provider := creds.Provider
	if provider == "" {
		provider = i.cfg.DefaultProvider
	}

	switch provider {
	case "twilio":
		var client *TwilioClient
		if creds.IsSet() {
			// accountSID, authToken (=APIKey), fromPhone (=CallerID)
			client = NewTwilioClient(creds.AccountSID, creds.APIKey, creds.CallerID)
		} else {
			client = i.twilio
		}
		return client.Hangup(ctx, callSid)
	default: // exotel
		if !creds.IsSet() {
			return fmt.Errorf("no Exotel credentials configured for this campaign")
		}
		client := NewExotelClient(creds.APIKey, creds.APIToken, creds.AccountSID, creds.CallerID, creds.AppID, creds.AppType)
		return client.Hangup(ctx, callSid)
	}
}
