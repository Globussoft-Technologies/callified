package dial

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"go.uber.org/zap"

	"github.com/globussoft/callified-backend/internal/callguard"
	"github.com/globussoft/callified-backend/internal/config"
	"github.com/globussoft/callified-backend/internal/db"
	rstore "github.com/globussoft/callified-backend/internal/redis"
	"github.com/globussoft/callified-backend/internal/webhook"
)

// CallData holds the information needed to initiate one outbound call.
type CallData struct {
	LeadID                 int64
	LeadName               string
	LeadPhone              string
	CampaignID             int64
	OrgID                  int64
	Interest               string
	Language               string
	TTSProvider            string
	TTSVoiceID             string
	TTSLanguage            string
	MaxCallDurationSeconds int
	// IsBridge=true routes the call to browser-to-phone mode: the Exotel stream is
	// relayed to the agent's browser WebSocket instead of the AI pipeline.
	IsBridge bool
	// UserEmail identifies the agent who clicked the call button.
	UserEmail string
	// UserID is the authenticated dashboard user placing the call. When non-zero
	// and the user owns a personal provider account, the initiator prefers that
	// account over the campaign/org default so agent-initiated calls go out from
	// the agent's own credentials.
	UserID int64
	// ExotelAccountID overrides the campaign's default provider account for this
	// specific call. 0 means use the campaign default (used by AI/server calls).
	ExotelAccountID int64
}

// Initiator orchestrates the full dial sequence:
// DND check → TRAI hours → Redis pending call → provider dial → DB log.
type Initiator struct {
	cfg    *config.Config
	store  *rstore.Store
	db     *db.DB
	disp   *webhook.Dispatcher
	exotel *ExotelClient
	tata   *TataClient
	twilio *TwilioClient
	log    *zap.Logger
	// prepareLive starts a Gemini Live session before a carrier call is answered.
	// Its returned function binds the session to the carrier call SID, or
	// cancels it when called with an empty SID after a failed dial.
	prepareLive func(context.Context, CallData) func(string)
}

// SetLivePreparer is wired by the WebSocket handler after both components are
// constructed. It is invoked for non-bridge Gemini Live carrier calls.
func (i *Initiator) SetLivePreparer(prepare func(context.Context, CallData) func(string)) {
	i.prepareLive = prepare
}

// New creates an Initiator wired to the supported telephony providers.
func New(cfg *config.Config, store *rstore.Store, database *db.DB, disp *webhook.Dispatcher, log *zap.Logger) *Initiator {
	return &Initiator{
		cfg:    cfg,
		store:  store,
		db:     database,
		disp:   disp,
		exotel: NewExotelClient(cfg.ExotelAPIKey, cfg.ExotelAPIToken, cfg.ExotelAccountSID, cfg.ExotelCallerID, cfg.ExotelAppID, "", cfg.ExotelRegion, cfg.ExotelSubdomain),
		tata:   NewTataClient(cfg.TataAPIKey, cfg.TataAPIToken, cfg.TataCallerID, cfg.TataAgentNumber, cfg.TataAPIEndpoint),
		twilio: NewTwilioClient(cfg.TwilioAccountSID, cfg.TwilioAuthToken, cfg.TwilioPhone),
		log:    log,
	}
}

// ErrDND is returned when the lead is on the DND list.
var ErrDND = fmt.Errorf("lead is on DND list")

// ErrCallHours is returned when TRAI calling hours are not active.
var ErrCallHours = fmt.Errorf("outside TRAI calling hours (9 AM – 9 PM)")

// ErrInsufficientCredits is returned when the org's prepaid balance is zero
// or negative. Surfaced to the API handler so it can return HTTP 402 with a
// minute-balance message instead of letting Exotel be charged for a
// dial we can't bill the customer for.
var ErrInsufficientCredits = fmt.Errorf("Credits exhausted")

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
	// Every real dial path uses the same minute balance gate, so calls stop
	// once available minutes are exhausted.
	if data.OrgID > 0 {
		oc, ocErr := i.db.GetOrgCredit(data.OrgID)
		if ocErr != nil {
			i.log.Warn("dial: GetOrgCredit failed; allowing call", zap.Error(ocErr))
		} else if oc != nil && oc.BalancePaise <= 0 {
			_ = i.db.UpdateLeadStatus(data.LeadID, "Insufficient Credits")
			i.store.EmitCampaignEvent(ctx, data.CampaignID, data.LeadName, data.LeadPhone,
				"failed", ErrInsufficientCredits.Error())
			return "", ErrInsufficientCredits
		}
	}

	// 3. Store pending call info in Redis (wshandler reads this on stream connect)
	pending := rstore.PendingCallInfo{
		Name:                   data.LeadName,
		Phone:                  data.LeadPhone,
		LeadID:                 data.LeadID,
		OrgID:                  data.OrgID,
		Interest:               data.Interest,
		CampaignID:             data.CampaignID,
		TTSProvider:            data.TTSProvider,
		TTSVoiceID:             data.TTSVoiceID,
		TTSLanguage:            data.TTSLanguage,
		MaxCallDurationSeconds: data.MaxCallDurationSeconds,
		IsBridge:               data.IsBridge,
		UserEmail:              data.UserEmail,
		UserID:                 data.UserID,
	}

	// 4. Resolve provider credentials.
	// Browser calls may override the campaign default with a per-machine/org
	// account so multiple systems can dial in parallel. The override is scoped
	// to the org and validated to be a voicebot account for bridge calls.
	// When UserID is set (agent/team-leader placed the call), an explicit
	// ExotelAccountID must belong either to the org or to that user; otherwise
	// we prefer the user's personal provider account before falling back to the
	// campaign/org default.
	var creds db.ExotelCreds
	if data.ExotelAccountID > 0 {
		var lookupErr error
		if data.UserID > 0 {
			creds, lookupErr = i.db.GetOrgOrUserExotelAccountCreds(data.ExotelAccountID, data.OrgID, data.UserID)
		} else {
			creds, lookupErr = i.db.GetOrgExotelAccountCreds(data.ExotelAccountID, data.OrgID)
		}
		if lookupErr != nil {
			return "", fmt.Errorf("lookup provider account: %w", lookupErr)
		}
		if err := validateTataAccountReadiness(creds); err != nil {
			return "", err
		}
		if !creds.IsSet() {
			return "", fmt.Errorf("provider account not found or inaccessible")
		}
		if creds.Direction == "inbound" {
			return "", fmt.Errorf("selected provider account is inbound-only; choose an outbound account")
		}
		isTataProvider := creds.Provider == "tata" || creds.Provider == "smartflo" || creds.Provider == "tata_tele"
		if data.IsBridge && !isTataProvider && creds.Provider != "plivo" && creds.AppType != "voicebot" {
			return "", fmt.Errorf("selected provider account is not a voicebot account; browser calls require app_type=voicebot")
		}
	}
	if !creds.IsSet() && data.UserID > 0 {
		if c, cerr := i.db.GetUserExotelAccountCreds(data.UserID, data.OrgID); cerr == nil {
			if err := validateTataAccountReadiness(c); err != nil {
				return "", err
			}
			if c.IsSet() {
				creds = c
			}
		} else if cerr != nil {
			return "", fmt.Errorf("lookup user provider account: %w", cerr)
		}
	}
	if !creds.IsSet() && data.CampaignID > 0 {
		if c, cerr := i.db.GetCampaignExotelCreds(data.CampaignID); cerr == nil {
			creds = c
		}
	}
	if err := validateTataAccountReadiness(creds); err != nil {
		return "", err
	}
	if creds.Direction == "inbound" {
		return "", fmt.Errorf("campaign provider account is inbound-only; choose an outbound account")
	}
	provider := creds.Provider
	if provider == "" {
		provider = i.cfg.DefaultProvider
	}
	// Carry the Exotel app/flow type and account choice through to the webhook
	// and hangup path so they use the same credentials used to place the call.
	pending.AppType = creds.AppType
	if creds.AccountID > 0 {
		pending.ExotelAccountID = creds.AccountID
	}
	var callSid string
	var bindPrepared func(string)
	preparedBound := false
	if provider != "twilio" && !data.IsBridge && i.prepareLive != nil &&
		(strings.EqualFold(data.TTSProvider, "gemini_live") || strings.TrimSpace(data.TTSProvider) == "") {
		bindPrepared = i.prepareLive(ctx, data)
		defer func() {
			if bindPrepared != nil && !preparedBound {
				bindPrepared("")
			}
		}()
	}

	switch provider {
	case "plivo":
		if !creds.IsSet() || creds.AccountID == 0 {
			return "", fmt.Errorf("select a saved Plivo provider account")
		}
		if !strings.HasPrefix(i.cfg.PublicServerURL, "https://") {
			return "", fmt.Errorf("Plivo requires an HTTPS PUBLIC_SERVER_URL for callbacks and audio streaming")
		}
		base := fmt.Sprintf("%s/webhook/plivo/", strings.TrimRight(i.cfg.PublicServerURL, "/"))
		var nonce [16]byte
		if _, err = rand.Read(nonce[:]); err != nil {
			return "", fmt.Errorf("create Plivo call identity: %w", err)
		}
		callSid = "plivo_" + hex.EncodeToString(nonce[:])
		pending.PlivoContextID = callSid
		if _, err = i.db.SaveCallLog(data.LeadID, data.CampaignID, data.OrgID, data.UserID, callSid, provider, data.LeadPhone, "initiated"); err != nil {
			return "", fmt.Errorf("save Plivo call log: %w", err)
		}
		if err = i.store.SetPendingCall(ctx, callSid, pending); err != nil {
			_ = i.db.UpdateCallLogStatus(callSid, "failed")
			return "", fmt.Errorf("save Plivo call context: %w", err)
		}
		query := fmt.Sprintf("?org_id=%d&account_id=%d&context_id=%s", data.OrgID, creds.AccountID, callSid)
		var requestID string
		requestID, err = NewPlivoClient(creds.AccountSID, creds.APIToken).InitiateCall(ctx, creds.CallerID, data.LeadPhone, base+"answer"+query, base+"status"+query)
		if err == nil {
			// Separate alias avoids overwriting context updated by a fast callback.
			if saveErr := i.store.SetPendingCall(ctx, "plivo-request:"+callSid, rstore.PendingCallInfo{PlivoRequestID: requestID}); saveErr != nil {
				i.log.Warn("save Plivo request alias", zap.Error(saveErr))
			}
		} else {
			_ = i.db.UpdateCallLogStatus(callSid, "failed")
		}
	case "twilio":
		return "", fmt.Errorf("Twilio provider is disabled; choose Exotel or Tata Tele")
	case "tata", "smartflo", "tata_tele":
		var tataClient *TataClient
		if creds.IsSet() {
			tataClient = NewTataClient(creds.APIKey, creds.APIToken, creds.CallerID, creds.AppID, creds.Subdomain)
		} else {
			tataClient = i.tata
		}
		statusURL := fmt.Sprintf("%s/webhook/tata/status?lead_id=%d&campaign_id=%d",
			i.cfg.PublicServerURL, data.LeadID, data.CampaignID)
		streamURL := tataStreamURL(i.cfg.PublicServerURL, data.LeadID, data.CampaignID, data.OrgID)
		callSid, err = tataClient.InitiateCall(ctx, data.LeadPhone, statusURL, streamURL)
	default: // exotel
		if !creds.IsSet() {
			i.store.EmitCampaignEvent(ctx, data.CampaignID, data.LeadName, data.LeadPhone, "failed", "no campaign Exotel credentials set")
			return "", fmt.Errorf("no Exotel credentials configured for this campaign")
		}
		exotelClient := NewExotelClient(creds.APIKey, creds.APIToken, creds.AccountSID, creds.CallerID, creds.AppID, creds.AppType, creds.Region, creds.Subdomain)
		statusURL := fmt.Sprintf("%s/webhook/exotel/status?lead_id=%d&campaign_id=%d",
			i.cfg.PublicServerURL, data.LeadID, data.CampaignID)
		callSid, err = exotelClient.InitiateCall(ctx, data.LeadPhone, "", statusURL)
	}
	if bindPrepared != nil && err == nil && callSid != "" {
		bindPrepared(callSid)
		preparedBound = true
	}
	if err != nil {
		_ = i.db.UpdateLeadStatus(data.LeadID, fmt.Sprintf("Call Failed (%s)", provider))
		// Live-feed: surface the dial-time failure (bad params, provider
		// rejected, etc.) on the campaign detail page.
		i.store.EmitCampaignEvent(ctx, data.CampaignID, data.LeadName, data.LeadPhone, "failed", fmt.Sprintf("%s: %v", provider, err))
		return "", fmt.Errorf("dial %s: %w", provider, err)
	}

	// 5. Persist pending call under the call SID for webhook lookup
	if provider != "plivo" {
		pending.ExotelCallSid = callSid
		if storeErr := i.store.SetPendingCall(ctx, callSid, pending); storeErr != nil {
			i.log.Warn("dial: SetPendingCall failed", zap.Error(storeErr))
		}
		// Also store under "latest" for fallback in wshandler
		_ = i.store.SetPendingCall(ctx, "latest", pending)

		// 6. Log dial attempt in DB
		if _, dbErr := i.db.SaveCallLog(data.LeadID, data.CampaignID, data.OrgID, data.UserID,
			callSid, provider, data.LeadPhone, "initiated"); dbErr != nil {
			i.log.Warn("dial: SaveCallLog failed", zap.Error(dbErr))
		}
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

func validateTataAccountReadiness(creds db.ExotelCreds) error {
	if (creds.Provider == "tata" || creds.Provider == "smartflo" || creds.Provider == "tata_tele") && creds.Direction != "inbound" && strings.TrimSpace(creds.APIToken) == "" {
		return fmt.Errorf("Tata provider account needs a Smartflo API token; update the account with its Smartflo token and Click-to-Call Support key")
	}
	return nil
}

func tataStreamURL(publicURL string, leadID, campaignID, orgID int64) string {
	base := strings.TrimRight(publicURL, "/")
	switch {
	case strings.HasPrefix(base, "https://"):
		base = "wss://" + strings.TrimPrefix(base, "https://")
	case strings.HasPrefix(base, "http://"):
		base = "ws://" + strings.TrimPrefix(base, "http://")
	}
	u, err := url.Parse(base + "/media-stream/tata")
	if err != nil {
		return base + "/media-stream/tata"
	}
	q := u.Query()
	q.Set("provider", "tata")
	if leadID > 0 {
		q.Set("lead_id", fmt.Sprint(leadID))
	}
	if campaignID > 0 {
		q.Set("campaign_id", fmt.Sprint(campaignID))
	}
	if orgID > 0 {
		q.Set("org_id", fmt.Sprint(orgID))
	}
	u.RawQuery = q.Encode()
	return u.String()
}

// Hangup ends an in-progress carrier call. It first tries to use the same
// provider account that placed the call (stored in Redis under the call SID),
// then falls back to the campaign-linked account. This keeps per-machine
// browser-call overrides consistent for the whole call lifecycle.
func (i *Initiator) Hangup(ctx context.Context, callSid string, campaignID int64) error {
	if callSid == "" {
		return fmt.Errorf("missing call sid")
	}

	var creds db.ExotelCreds
	// 1. Per-call override from the Redis pending entry.
	if pending, ok := i.store.GetPendingCall(ctx, callSid); ok && pending.ExotelAccountID > 0 {
		if c, cerr := i.db.GetOrgOrUserExotelAccountCreds(pending.ExotelAccountID, pending.OrgID, 0); cerr == nil && c.IsSet() {
			creds = c
		}
	}
	// 2. Campaign default.
	if !creds.IsSet() && campaignID > 0 {
		creds, _ = i.db.GetCampaignExotelCreds(campaignID)
	}

	provider := creds.Provider
	if provider == "" {
		provider = i.cfg.DefaultProvider
	}

	switch provider {
	case "plivo":
		if !creds.IsSet() {
			return fmt.Errorf("Plivo credentials unavailable")
		}
		if pending, ok := i.store.GetPendingCall(ctx, callSid); ok {
			if pending.PlivoContextID != "" {
				if canonical, found := i.store.GetPendingCall(ctx, pending.PlivoContextID); found {
					pending = canonical
				}
			}
			if pending.ExotelCallSid == "" {
				request := pending.PlivoRequestID
				if request == "" {
					alias, _ := i.store.GetPendingCall(ctx, "plivo-request:"+pending.PlivoContextID)
					request = alias.PlivoRequestID
				}
				return NewPlivoClient(creds.AccountSID, creds.APIToken).CancelRequest(ctx, request)
			}
			callSid = pending.ExotelCallSid
		}
		return NewPlivoClient(creds.AccountSID, creds.APIToken).Hangup(ctx, callSid)
	case "twilio":
		return fmt.Errorf("Twilio provider is disabled; choose Exotel or Tata Tele")
	case "tata", "smartflo", "tata_tele":
		var client *TataClient
		if creds.IsSet() {
			client = NewTataClient(creds.APIKey, creds.APIToken, creds.CallerID, creds.AppID, creds.Subdomain)
		} else {
			client = i.tata
		}
		return client.Hangup(ctx, callSid)
	default: // exotel
		if !creds.IsSet() {
			return fmt.Errorf("no Exotel credentials configured for this campaign")
		}
		client := NewExotelClient(creds.APIKey, creds.APIToken, creds.AccountSID, creds.CallerID, creds.AppID, creds.AppType, creds.Region, creds.Subdomain)
		return client.Hangup(ctx, callSid)
	}
}
