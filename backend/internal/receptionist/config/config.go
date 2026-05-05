// Package config loads runtime settings from environment variables.
//
// We use a single Settings struct populated lazily so the rest of the
// codebase can rely on `config.Get()` rather than re-reading os.Getenv
// at every site.
package config

import (
	"bufio"
	"os"
	"strconv"
	"strings"
	"sync"
)

// Settings is the canonical configuration object.
type Settings struct {
	AppEnv            string
	LogLevel          string
	SessionTTLSeconds int

	// Twilio (optional).
	TwilioAccountSID          string
	TwilioAuthToken           string
	TwilioPhoneNumber         string
	EmergencyEscalationNumber string
	PublicBaseURL             string
	TwilioVoiceFemale         string
	TwilioVoiceMale           string
	TwilioVoiceDefault        string

	// Clinic profile.
	ClinicName    string
	ClinicHours   string
	ClinicAddress string
	ClinicPhone   string

	// Emergency / ambulance.
	AmbulanceDispatchEnabled bool

	// Minimum latency in milliseconds for non-emergency turns. The
	// conversation manager sleeps the remainder so each reply takes at
	// least this long, simulating a more natural pacing. Emergency turns
	// always bypass this — delaying safety guidance is dangerous.
	MinTurnLatencyMS int

	// LLM (Claude).
	AnthropicAPIKey string
	LLMModel        string
	LLMMaxTokens    int
}

var (
	once    sync.Once
	current *Settings
)

// Get returns the cached Settings. First call loads the .env file (if
// present) and reads from the environment.
func Get() *Settings {
	once.Do(func() {
		loadDotEnv(".env")
		current = &Settings{
			AppEnv:                    getenvDefault("APP_ENV", "development"),
			LogLevel:                  getenvDefault("LOG_LEVEL", "INFO"),
			SessionTTLSeconds:         getenvInt("SESSION_TTL_SECONDS", 1800),
			TwilioAccountSID:          os.Getenv("TWILIO_ACCOUNT_SID"),
			TwilioAuthToken:           os.Getenv("TWILIO_AUTH_TOKEN"),
			TwilioPhoneNumber:         os.Getenv("TWILIO_PHONE_NUMBER"),
			EmergencyEscalationNumber: os.Getenv("EMERGENCY_ESCALATION_NUMBER"),
			PublicBaseURL:             os.Getenv("PUBLIC_BASE_URL"),
			TwilioVoiceFemale:         getenvDefault("TWILIO_VOICE_FEMALE", "Polly.Joanna"),
			TwilioVoiceMale:           getenvDefault("TWILIO_VOICE_MALE", "Polly.Matthew"),
			TwilioVoiceDefault:        getenvDefault("TWILIO_VOICE_DEFAULT", "female"),
			ClinicName:                getenvDefault("CLINIC_NAME", "Sunrise Family Clinic"),
			ClinicHours:               getenvDefault("CLINIC_HOURS", "Monday to Friday, 8 AM to 6 PM; Saturday, 9 AM to 1 PM"),
			ClinicAddress:             getenvDefault("CLINIC_ADDRESS", "123 Wellness Avenue, Springfield, IL"),
			ClinicPhone:               getenvDefault("CLINIC_PHONE", "+15551112222"),
			AmbulanceDispatchEnabled:  getenvBool("AMBULANCE_DISPATCH_ENABLED", true),
			MinTurnLatencyMS:          getenvInt("MIN_TURN_LATENCY_MS", 3000),
			AnthropicAPIKey:           os.Getenv("ANTHROPIC_API_KEY"),
			LLMModel:                  getenvDefault("LLM_MODEL", "claude-opus-4-7"),
			LLMMaxTokens:              getenvInt("LLM_MAX_TOKENS", 1024),
		}
	})
	return current
}

func getenvDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getenvInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func getenvBool(key string, def bool) bool {
	if v := os.Getenv(key); v != "" {
		switch strings.ToLower(v) {
		case "1", "true", "yes", "on":
			return true
		case "0", "false", "no", "off":
			return false
		}
	}
	return def
}

// loadDotEnv populates os.Setenv from KEY=VALUE pairs in `path`. Missing
// file is not an error. Existing env vars take precedence over file
// values, matching python-dotenv's default behavior.
func loadDotEnv(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		eq := strings.IndexByte(line, '=')
		if eq <= 0 {
			continue
		}
		key := strings.TrimSpace(line[:eq])
		val := strings.TrimSpace(line[eq+1:])
		// Strip surrounding quotes.
		if len(val) >= 2 && (val[0] == '"' && val[len(val)-1] == '"' ||
			val[0] == '\'' && val[len(val)-1] == '\'') {
			val = val[1 : len(val)-1]
		}
		if _, exists := os.LookupEnv(key); !exists {
			os.Setenv(key, val)
		}
	}
}
