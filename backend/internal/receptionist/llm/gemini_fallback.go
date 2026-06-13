package llm

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/globussoft/callified-backend/internal/llm"
	"github.com/globussoft/callified-backend/internal/receptionist/appointment"
	"github.com/globussoft/callified-backend/internal/receptionist/config"
	"github.com/globussoft/callified-backend/internal/receptionist/session"
)

// GeminiFallback provides conversational replies for the rule-based path
// when no Anthropic key is set. It uses callified's existing GeminiClient
// (so we don't add another dependency or another API key) and constrains
// the model to the static clinic facts — clinic name, hours, address,
// phone, and the appointment service's doctor roster. The system prompt
// explicitly forbids inventing names, prices, services, or hours.
//
// Wiring is lazy: if GEMINI_API_KEY is unset, Reply returns "" so the
// caller can fall back to its existing canned response.
type GeminiFallback struct {
	apptSvc *appointment.Service
	client  *llm.GeminiClient
	once    sync.Once
}

// NewGeminiFallback returns a GeminiFallback that pulls the doctor roster
// from apptSvc on every call. The client is lazily initialized on first
// Reply() so package init has no side effects.
func NewGeminiFallback(apptSvc *appointment.Service) *GeminiFallback {
	return &GeminiFallback{apptSvc: apptSvc}
}

// Reply returns a single conversational sentence for userText, given the
// current session history. Returns "" when GEMINI_API_KEY is unset or the
// API errors — the caller should treat empty as "fall back to canned".
func (g *GeminiFallback) Reply(ctx context.Context, sess *session.Session, userText string) string {
	g.once.Do(func() {
		key := os.Getenv("GEMINI_API_KEY")
		if key == "" {
			return
		}
		// gemini-2.5-flash matches the model callified's main backend uses
		// (GEMINI_MODEL in .env). 2.0-flash returns 429 quota-exceeded on
		// this account; 2.5-flash has working free-tier quota.
		model := os.Getenv("GEMINI_MODEL")
		if model == "" {
			model = "gemini-2.5-flash"
		}
		g.client = llm.NewGeminiClient(key, model)
	})
	if g.client == nil {
		return ""
	}

	cfg := config.Get()

	// Build the static facts block. The list of doctors comes from the
	// in-memory appointment service — this is the single source of truth;
	// the LLM must not invent any others.
	var doctors []string
	for _, d := range g.apptSvc.AvailableDoctors() {
		doctors = append(doctors, fmt.Sprintf("- %s (%s)", d.Name, d.Specialty))
	}
	system := fmt.Sprintf(`You are an AI receptionist for %s. You are on a phone call with a caller. Reply in ONE short, natural sentence — no lists, no markdown, no technical jargon. The caller will hear your reply through text-to-speech.

Static facts (the only ones you may state):
- Clinic name: %s
- Hours: %s
- Address: %s
- Phone: %s
- Doctors:
%s

Hard rules:
- NEVER invent a doctor, specialty, price, service, hour, or address that is not in the list above.
- If the caller asks something we do not have an answer for (e.g. parking, insurance plans, lab results), say a senior team member will get back to them and offer to take a callback number.
- If the caller wants to book, reschedule, or cancel an appointment, briefly invite them to do so — the booking system handles the actual scheduling.
- If the caller is wrapping up ("no thanks", "goodbye", "that's all"), give a short warm sign-off.

Tone: warm, concise, professional — like a smart human receptionist on a busy day.`,
		cfg.ClinicName, cfg.ClinicName, cfg.ClinicHours, cfg.ClinicAddress,
		cfg.ClinicPhone, strings.Join(doctors, "\n"))

	// Build a compact user-message that includes the last few turns plus
	// the current utterance — Gemini doesn't have a native multi-turn
	// chat in the GenerateText helper, so we fold history into the prompt.
	var hist strings.Builder
	tail := sess.Transcript
	if len(tail) > 8 {
		tail = tail[len(tail)-8:]
	}
	for _, t := range tail {
		role := "Caller"
		if t.Role == "assistant" {
			role = "Receptionist"
		}
		fmt.Fprintf(&hist, "%s: %s\n", role, t.Text)
	}
	fmt.Fprintf(&hist, "Caller: %s\nReceptionist:", userText)

	cctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	reply, err := g.client.GenerateText(cctx, system, hist.String(), 200)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(reply)
}
