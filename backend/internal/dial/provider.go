package dial

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/globussoft/callified-backend/internal/config"
	"github.com/globussoft/callified-backend/internal/db"
)

// ProviderType identifies the telephony carrier implementation.
type ProviderType string

const (
	ProviderExotel ProviderType = "exotel"
	ProviderTwilio ProviderType = "twilio"
	ProviderTata   ProviderType = "tata"
)

// Provider represents a telephony carrier that can initiate outbound calls and
// accept inbound/webhook events. It abstracts Exotel, Twilio, Tata, and future
// carriers so the dial initiator can resolve and dial without carrier-specific
// logic.
type Provider interface {
	// Name returns the carrier identifier, e.g. "exotel", "twilio", "tata".
	Name() string

	// Direction returns "inbound", "outbound", "both", or "" depending on the
	// stored account configuration. Empty string is treated as "both".
	Direction() string

	// ValidateCredentials performs a lightweight validation of the stored
	// credentials. It may perform a read-only carrier API probe when possible,
	// but at minimum it checks that all required fields are present and that
	// the account can be used for outbound calls.
	ValidateCredentials(ctx context.Context) error

	// InitiateCall places an outbound AI/media-stream call to the customer
	// phone number and returns the carrier-issued call SID.
	InitiateCall(ctx context.Context, toPhone, flowURL, callbackURL string) (string, error)

	// InitiateHumanCall places a two-party (agent + customer) bridge call and
	// returns the carrier-issued call SID. Not all carriers support this.
	InitiateHumanCall(ctx context.Context, agentPhone, customerPhone, callbackURL string) (string, error)

	// Hangup terminates an in-progress call.
	Hangup(ctx context.Context, callSid string) error
}

// ProviderAccount is the carrier-agnostic representation of a stored provider
// account. It mirrors the shape of db.ExotelCreds and db.TwilioCreds.
type ProviderAccount struct {
	ID          int64
	Name        string
	Type        ProviderType
	APIKey      string
	APIToken    string
	AccountSID  string
	AccountID   string
	CallerID    string
	AppID       string
	AppType     string
	Region      string
	Subdomain   string
	Direction   string
	IsGlobal    bool
	IsVoicebot  bool
	APIEndpoint string
}

// IsOutbound returns true if the account can be used for outbound calls.
func (a ProviderAccount) IsOutbound() bool {
	return a.Direction == "outbound" || a.Direction == "both" || a.Direction == ""
}

// Validate returns an error if the account cannot place calls.
func (a ProviderAccount) Validate() error {
	if a.Type == "" {
		return fmt.Errorf("provider type is required")
	}
	switch a.Type {
	case ProviderExotel:
		if a.AccountSID == "" || a.APIKey == "" || a.APIToken == "" || a.CallerID == "" || a.AppID == "" {
			return fmt.Errorf("exotel account is missing required fields (account SID, API key, API token, caller ID, or app ID)")
		}
	case ProviderTwilio:
		if a.AccountSID == "" || a.APIToken == "" || a.CallerID == "" {
			return fmt.Errorf("twilio account is missing required fields (account SID, auth token, or caller ID)")
		}
	case ProviderTata:
		if a.APIToken == "" || a.CallerID == "" || a.APIEndpoint == "" {
			return fmt.Errorf("tata account is missing required fields (API token, caller ID, or endpoint)")
		}
	default:
		return fmt.Errorf("unsupported provider type: %s", a.Type)
	}
	if !a.IsOutbound() {
		return fmt.Errorf("selected provider account is %s-only; choose an outbound account", a.Direction)
	}
	return nil
}

// exotelProvider wraps ExotelClient to satisfy the Provider interface.
type exotelProvider struct {
	client    *ExotelClient
	direction string
}

func (p *exotelProvider) Name() string      { return string(ProviderExotel) }
func (p *exotelProvider) Direction() string { return p.direction }

func (p *exotelProvider) ValidateCredentials(ctx context.Context) error {
	if p.client == nil {
		return fmt.Errorf("exotel client is not initialized")
	}
	// Minimal structural validation. A deeper check could call
	// https://api.exotel.com/v1/Accounts/{sid} in the future.
	if p.client.accountSID == "" || p.client.apiKey == "" || p.client.apiToken == "" {
		return fmt.Errorf("exotel credentials are incomplete")
	}
	if p.client.callerID == "" || p.client.appID == "" {
		return fmt.Errorf("exotel caller ID or app ID is missing")
	}
	return nil
}

func (p *exotelProvider) InitiateCall(ctx context.Context, toPhone, flowURL, callbackURL string) (string, error) {
	return p.client.InitiateCall(ctx, toPhone, flowURL, callbackURL)
}

func (p *exotelProvider) InitiateHumanCall(ctx context.Context, agentPhone, customerPhone, callbackURL string) (string, error) {
	return p.client.InitiateHumanCall(ctx, agentPhone, customerPhone, callbackURL)
}

func (p *exotelProvider) Hangup(ctx context.Context, callSid string) error {
	return p.client.Hangup(ctx, callSid)
}

// twilioProvider wraps TwilioClient to satisfy the Provider interface.
type twilioProvider struct {
	client    *TwilioClient
	direction string
}

func (p *twilioProvider) Name() string      { return string(ProviderTwilio) }
func (p *twilioProvider) Direction() string { return p.direction }

func (p *twilioProvider) ValidateCredentials(ctx context.Context) error {
	if p.client == nil {
		return fmt.Errorf("twilio client is not initialized")
	}
	if p.client.accountSID == "" || p.client.authToken == "" || p.client.fromPhone == "" {
		return fmt.Errorf("twilio credentials are incomplete")
	}
	// Lightweight probe: fetch account metadata.
	endpoint := fmt.Sprintf("https://api.twilio.com/2010-04-01/Accounts/%s.json", p.client.accountSID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return fmt.Errorf("twilio: build validation request: %w", err)
	}
	req.SetBasicAuth(p.client.accountSID, p.client.authToken)
	resp, err := p.client.client.Do(req)
	if err != nil {
		return fmt.Errorf("twilio: validation request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		return fmt.Errorf("twilio: invalid account SID or auth token (401)")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("twilio: validation failed with status %d", resp.StatusCode)
	}
	return nil
}

func (p *twilioProvider) InitiateCall(ctx context.Context, toPhone, flowURL, callbackURL string) (string, error) {
	return p.client.InitiateCall(ctx, toPhone, flowURL, callbackURL)
}

func (p *twilioProvider) InitiateHumanCall(ctx context.Context, agentPhone, customerPhone, callbackURL string) (string, error) {
	return "", fmt.Errorf("twilio: two-party human bridge is not implemented")
}

func (p *twilioProvider) Hangup(ctx context.Context, callSid string) error {
	return p.client.Hangup(ctx, callSid)
}

// tataProvider wraps TataClient to satisfy the Provider interface.
type tataProvider struct {
	client    *TataClient
	direction string
}

func (p *tataProvider) Name() string      { return string(ProviderTata) }
func (p *tataProvider) Direction() string { return p.direction }

func (p *tataProvider) ValidateCredentials(ctx context.Context) error {
	if p.client == nil {
		return fmt.Errorf("tata client is not initialized")
	}
	if !p.client.IsSet() {
		return fmt.Errorf("tata credentials are incomplete")
	}
	return nil
}

func (p *tataProvider) InitiateCall(ctx context.Context, toPhone, flowURL, callbackURL string) (string, error) {
	return p.client.InitiateCall(ctx, toPhone, callbackURL, flowURL)
}

func (p *tataProvider) InitiateHumanCall(ctx context.Context, agentPhone, customerPhone, callbackURL string) (string, error) {
	return "", fmt.Errorf("tata: two-party human bridge is not implemented")
}

func (p *tataProvider) Hangup(ctx context.Context, callSid string) error {
	return p.client.Hangup(ctx, callSid)
}

// NewProvider creates a Provider from a ProviderAccount. It returns a structured
// error if the account is missing fields or is not outbound-capable.
func NewProvider(acc ProviderAccount) (Provider, error) {
	if err := acc.Validate(); err != nil {
		return nil, err
	}
	switch acc.Type {
	case ProviderExotel:
		client := NewExotelClient(
			acc.APIKey,
			acc.APIToken,
			acc.AccountSID,
			acc.CallerID,
			acc.AppID,
			acc.AppType,
			acc.Region,
			acc.Subdomain,
		)
		return &exotelProvider{client: client, direction: acc.Direction}, nil
	case ProviderTwilio:
		// ExotelCreds field mapping for Twilio:
		//   APIKey     -> Auth Token
		//   APIToken   -> API Key SID (not used for basic auth)
		//   AccountSID -> Account SID
		//   CallerID   -> From phone number
		client := NewTwilioClient(acc.AccountSID, acc.APIKey, acc.CallerID)
		return &twilioProvider{client: client, direction: acc.Direction}, nil
	case ProviderTata:
		// ExotelCreds field mapping for Tata:
		//   APIKey     -> API token
		//   CallerID   -> Caller ID
		//   Subdomain  -> API endpoint
		client := NewTataClient(acc.APIKey, acc.CallerID, "", acc.Subdomain)
		return &tataProvider{client: client, direction: acc.Direction}, nil
	default:
		return nil, fmt.Errorf("unsupported provider type: %s", acc.Type)
	}
}

// ProviderAccountFromExotelCreds converts a db.ExotelCreds (the legacy struct
// used across campaigns and provider accounts) into a carrier-agnostic
// ProviderAccount. This allows Initiator to use NewProvider without rewriting
// the DB layer.
func ProviderAccountFromExotelCreds(creds db.ExotelCreds) ProviderAccount {
	ptype := ProviderType(strings.ToLower(creds.Provider))
	if ptype == "" {
		ptype = ProviderExotel
	}
	if ptype == "smartflo" || ptype == "tata_tele" {
		ptype = ProviderTata
	}
	return ProviderAccount{
		ID:         creds.AccountID,
		Type:       ptype,
		APIKey:     creds.APIKey,
		APIToken:   creds.APIToken,
		AccountSID: creds.AccountSID,
		CallerID:   creds.CallerID,
		AppID:      creds.AppID,
		AppType:    creds.AppType,
		Region:     creds.Region,
		Subdomain:  creds.Subdomain,
		Direction:  creds.Direction,
	}
}

// NewProviderFromConfig builds the legacy config-level provider from
// environment variables. It is used as a fallback when no stored provider
// account is selected for a campaign.
func NewProviderFromConfig(cfg *config.Config) (Provider, error) {
	provider := strings.ToLower(cfg.DefaultProvider)
	if provider == "" {
		provider = string(ProviderExotel)
	}
	switch provider {
	case string(ProviderExotel):
		acc := ProviderAccount{
			Type:      ProviderExotel,
			APIKey:    cfg.ExotelAPIKey,
			APIToken:  cfg.ExotelAPIToken,
			AccountSID: cfg.ExotelAccountSID,
			CallerID:  cfg.ExotelCallerID,
			AppID:     cfg.ExotelAppID,
			AppType:   "exoml", // legacy config-level accounts default to exoml
			Region:    cfg.ExotelRegion,
			Subdomain: cfg.ExotelSubdomain,
			Direction: "outbound",
		}
		return NewProvider(acc)
	case string(ProviderTata), "smartflo", "tata_tele":
		acc := ProviderAccount{
			Type:      ProviderTata,
			APIKey:    cfg.TataAPIToken,
			CallerID:  cfg.TataCallerID,
			Subdomain: cfg.TataAPIEndpoint,
			Direction: "outbound",
		}
		return NewProvider(acc)
	case string(ProviderTwilio):
		acc := ProviderAccount{
			Type:      ProviderTwilio,
			APIKey:    cfg.TwilioAuthToken,
			APIToken:  cfg.TwilioAuthToken, // not used by basic-auth client
			AccountSID: cfg.TwilioAccountSID,
			CallerID:  cfg.TwilioPhone,
			Direction: "outbound",
		}
		return NewProvider(acc)
	default:
		return nil, fmt.Errorf("unsupported default provider %q from config", cfg.DefaultProvider)
	}
}
