// Package conversation orchestrates one caller's dialog: emergency check,
// LLM dispatch (when configured), and the rule-based fallback FSM.
//
// The most important invariant: emergency keyword detection runs on
// EVERY utterance regardless of state and short-circuits both the LLM
// and the FSM. Never let the model decide whether "chest pain" is an
// emergency.
package conversation

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/globussoft/callified-backend/internal/receptionist/ambulance"
	"github.com/globussoft/callified-backend/internal/receptionist/appointment"
	"github.com/globussoft/callified-backend/internal/receptionist/config"
	"github.com/globussoft/callified-backend/internal/receptionist/emergency"
	"github.com/globussoft/callified-backend/internal/receptionist/intent"
	"github.com/globussoft/callified-backend/internal/receptionist/llm"
	"github.com/globussoft/callified-backend/internal/receptionist/models"
	"github.com/globussoft/callified-backend/internal/receptionist/session"
)

// Manager wires the dependencies and exposes the per-turn handler.
type Manager struct {
	store          *session.Store
	apptSvc        *appointment.Service
	ambSvc         *ambulance.Service
	llmAgent       *llm.Agent
	geminiFallback *llm.GeminiFallback // non-nil when GEMINI_API_KEY is set; used only when the rule-based FSM has no good answer
}

// New constructs a Manager.
func New(store *session.Store, apptSvc *appointment.Service,
	ambSvc *ambulance.Service, llmAgent *llm.Agent) *Manager {
	// Register roster first names with the intent extractor so bare
	// references like "book with Mike" resolve without the "Dr." prefix.
	names := make([]string, 0, len(apptSvc.AvailableDoctors()))
	for _, d := range apptSvc.AvailableDoctors() {
		names = append(names, d.Name)
	}
	intent.SetRosterFirstNames(names)
	return &Manager{
		store: store, apptSvc: apptSvc, ambSvc: ambSvc, llmAgent: llmAgent,
		// Built unconditionally — Reply() returns "" if GEMINI_API_KEY is
		// unset, so the call site can keep its existing canned fallback.
		geminiFallback: llm.NewGeminiFallback(apptSvc),
	}
}

// thanksRE matches a bare "thank you" / "thanks" / "thx" — but NOT
// when paired with "bye" / "goodbye" (those are already IntentGoodbye
// and would double-trigger the multi-thanks shutoff).
var thanksRE = regexp.MustCompile(`(?i)\bthan(?:k\s*you|ks|x|k\s*u)\b`)
var goodbyeRE = regexp.MustCompile(`(?i)\b(?:bye|goodbye)\b`)

func isThankYou(text string) bool {
	return thanksRE.MatchString(text) && !goodbyeRE.MatchString(text)
}

// lastUserText returns the most recent caller utterance from the session
// transcript — used by handlers that don't get text passed in directly.
func lastUserText(sess *session.Session) string {
	for i := len(sess.Transcript) - 1; i >= 0; i-- {
		if sess.Transcript[i].Role == "user" {
			return sess.Transcript[i].Text
		}
	}
	return ""
}

// llmFallbackOr returns the Gemini-generated conversational reply for the
// current turn when available, otherwise returns the provided canned
// string. Used at every "I don't know what they meant" branch in the
// rule-based FSM so generic questions ("do you do x-rays?", "how much
// does a visit cost?") get a real receptionist-style answer instead of
// "I can help with booking, rescheduling…".
func (m *Manager) llmFallbackOr(sess *session.Session, userText, canned string) string {
	if m.geminiFallback == nil {
		return canned
	}
	if reply := m.geminiFallback.Reply(context.Background(), sess, userText); reply != "" {
		return reply
	}
	return canned
}

// TurnResult is one assistant reply.
type TurnResult struct {
	Message     string
	State       models.ConversationState
	Intent      models.IntentType
	IsEmergency bool
	Metadata    map[string]any
}

// StartCall opens a new caller session and returns the opening greeting.
// agentName is optional — when set (e.g. the user picked the "Sarah" voice),
// the greeting introduces the AI by that name instead of the generic
// "the AI receptionist". Empty string preserves the legacy phrasing.
func (m *Manager) StartCall(callerID, language, agentName string) (*session.Session, string) {
	cfg := config.Get()
	sess := m.store.Create(callerID, language)
	intro := "the AI receptionist"
	if a := strings.TrimSpace(agentName); a != "" {
		intro = a
	}
	greeting := fmt.Sprintf(
		"Thank you for calling %s. This is %s. May I have "+
			"your name, please, and how can I help you today?",
		cfg.ClinicName, intro,
	)
	sess.Append("assistant", greeting)
	sess.State = models.StateAwaitingName
	return sess, greeting
}

// EndCall marks the session ended and returns it (for transcript reads).
func (m *Manager) EndCall(sessionID string) *session.Session {
	sess := m.store.Get(sessionID)
	if sess == nil {
		return nil
	}
	farewell := fmt.Sprintf("Thank you for calling %s. Take care, and have a great day.",
		config.Get().ClinicName)
	sess.Append("assistant", farewell)
	return m.store.End(sessionID)
}

// HandleTurn processes one caller utterance, optionally enforcing a
// minimum reply latency for natural pacing (configured via
// MIN_TURN_LATENCY_MS). Emergency turns always bypass the delay —
// delaying safety guidance is dangerous.
func (m *Manager) HandleTurn(sessionID, text string) *TurnResult {
	start := time.Now()
	res := m.handleTurnInner(sessionID, text)
	if res != nil && !res.IsEmergency {
		minLatency := time.Duration(config.Get().MinTurnLatencyMS) * time.Millisecond
		if remaining := minLatency - time.Since(start); remaining > 0 {
			time.Sleep(remaining)
		}
	}
	return res
}

func (m *Manager) handleTurnInner(sessionID, text string) *TurnResult {
	sess := m.store.Get(sessionID)
	if sess == nil {
		return nil
	}
	text = strings.TrimSpace(text)
	sess.Append("user", text)

	// 1. Emergency keyword guardrail — runs first, every state.
	if det := emergency.Detect(text); det.IsEmergency {
		return m.handleEmergency(sess, text, det)
	}

	// 1b. Post-booking thanks → end the call. Once an appointment has
	// been confirmed, a "thank you" / "thanks" is the caller signalling
	// they're done. Wrap up cleanly instead of looping back to "anything
	// else?". Outside of the post-booking window, thanks flows through
	// normally so callers can chat without being hung up on.
	if booked, _ := sess.Slots["booking_confirmed"].(bool); booked && isThankYou(text) {
		sess.State = models.StateEnded
		msg := fmt.Sprintf("You're welcome. Thank you for calling %s. Goodbye.",
			config.Get().ClinicName)
		sess.Append("assistant", msg)
		return &TurnResult{
			Message: msg, State: models.StateEnded,
			Intent: models.IntentGoodbye, Metadata: map[string]any{},
		}
	}

	// 2. Emergency follow-up state — caller giving address. Stays
	// rule-based for predictability; we don't free-form during an
	// active emergency.
	if sess.State == models.StateEmergency {
		return m.handleEmergencyState(sess, text)
	}

	// 3. Normal flow: LLM if enabled, else rule-based FSM.
	if m.llmAgent.Enabled() {
		res := m.llmAgent.Respond(sess)
		sess.Append("assistant", res.Reply)
		if res.EndCall {
			sess.State = models.StateEnded
		} else {
			sess.State = models.StateAwaitingFollowup
		}
		return &TurnResult{
			Message:  res.Reply,
			State:    sess.State,
			Intent:   models.IntentUnknown,
			Metadata: res.Metadata,
		}
	}

	return m.handleRuleBased(sess, text)
}

// --- Emergency -------------------------------------------------------

func (m *Manager) handleEmergency(sess *session.Session, text string, det emergency.Detection) *TurnResult {
	wasAlready := sess.State == models.StateEmergency
	sess.State = models.StateEmergency
	cfg := config.Get()

	var dispatch *ambulance.Dispatch
	if cfg.AmbulanceDispatchEnabled {
		patientName, _ := sess.Slots["patient_name"].(string)
		dispatch = m.ambSvc.Dispatch(ambulance.DispatchInput{
			SessionID:      sess.ID,
			CallerID:       sess.CallerID,
			PatientName:    patientName,
			MatchedPhrase:  det.MatchedPhrase,
			TranscriptTail: text,
		})
	}

	var msg string
	switch {
	case dispatch == nil:
		msg = "This sounds like an emergency. Please hang up and call 911 " +
			"immediately. If you are with the patient, stay on the line " +
			"with emergency services."
	case wasAlready:
		msg = fmt.Sprintf(
			"Help is already on the way. Vehicle %s with %s is en route — "+
				"ETA about %d minutes. Please stay on the line with 911. "+
				"Your reference is %s.",
			dispatch.VehicleID, dispatch.CrewLead, dispatch.ETAMinutes, dispatch.ID,
		)
	default:
		msg = fmt.Sprintf(
			"I have booked an ambulance for you. Vehicle %s with %s is on "+
				"the way — ETA about %d minutes. Your reference is %s. "+
				"Please also call 911 so a dispatcher can guide you. "+
				"What is the address where help is needed?",
			dispatch.VehicleID, dispatch.CrewLead, dispatch.ETAMinutes, dispatch.ID,
		)
	}
	sess.Append("assistant", msg)

	meta := map[string]any{"matched_phrase": det.MatchedPhrase}
	if dispatch != nil {
		meta["dispatch_id"] = dispatch.ID
		meta["dispatch_status"] = dispatch.Status
		meta["vehicle_id"] = dispatch.VehicleID
		meta["crew_lead"] = dispatch.CrewLead
		meta["eta_minutes"] = dispatch.ETAMinutes
		meta["location"] = dispatch.Location
	}
	return &TurnResult{
		Message: msg, State: sess.State, Intent: models.IntentEmergency,
		IsEmergency: true, Metadata: meta,
	}
}

func (m *Manager) handleEmergencyState(sess *session.Session, text string) *TurnResult {
	intentMatch := intent.Detect(text)
	dispatch := m.ambSvc.GetForSession(sess.ID)

	if intentMatch.Intent == models.IntentGoodbye {
		sess.State = models.StateEnded
		msg := "Stay safe. Help is on the way and 911 will be with you shortly."
		sess.Append("assistant", msg)
		meta := map[string]any{}
		if dispatch != nil {
			meta["dispatch_id"] = dispatch.ID
		}
		return &TurnResult{Message: msg, State: sess.State, Intent: intentMatch.Intent, Metadata: meta}
	}

	if dispatch == nil {
		msg := "Please call 911 immediately if you haven't already. Stay on the line with them."
		sess.Append("assistant", msg)
		return &TurnResult{Message: msg, State: sess.State, Intent: intentMatch.Intent, Metadata: map[string]any{}}
	}

	address := strings.TrimSpace(text)
	sess.Slots["emergency_address"] = address
	m.ambSvc.UpdateLocation(dispatch.ID, address)
	// Address captured — release the session back to normal flow so the
	// caller can ask follow-up questions while help is en route. The
	// emergency keyword check at the top of HandleTurn still re-fires
	// for any genuinely new emergency utterance.
	sess.State = models.StateAwaitingFollowup

	msg := fmt.Sprintf(
		"Thank you. I've forwarded '%s' to vehicle %s (%s). They are en "+
			"route now — ETA about %d minutes. Please stay on the line with "+
			"911 if you have already called them, and unlock the door if "+
			"you safely can. Is there anything else I can help with?",
		address, dispatch.VehicleID, dispatch.CrewLead, dispatch.ETAMinutes,
	)
	sess.Append("assistant", msg)
	return &TurnResult{
		Message: msg, State: sess.State, Intent: intentMatch.Intent,
		Metadata: map[string]any{
			"dispatch_id": dispatch.ID,
			"location":    address,
			"eta_minutes": dispatch.ETAMinutes,
			"vehicle_id":  dispatch.VehicleID,
		},
	}
}

// --- Rule-based FSM (fallback when no API key) -----------------------

var questionRE = regexp.MustCompile(`(?i)^\s*(?:what|who|which|can|could|would|do\s+you|tell|list|suggest|give|show|name)\b`)

func (m *Manager) handleRuleBased(sess *session.Session, text string) *TurnResult {
	intentMatch := intent.Detect(text)
	var (
		reply string
		meta  map[string]any
	)
	switch sess.State {
	case models.StateAwaitingName:
		reply, meta = m.handleAwaitingName(sess, text)
	case models.StateAwaitingPurpose:
		reply, meta = m.routePurpose(sess, text, intentMatch)
	case models.StateAppointmentBookDoctor:
		reply, meta = m.handleBookDoctor(sess, text, intentMatch)
	case models.StateAppointmentBookTime:
		reply, meta = m.handleBookTime(sess, intentMatch)
	case models.StateAppointmentRescheduleID:
		reply, meta = m.handleRescheduleID(sess, intentMatch)
	case models.StateAppointmentRescheduleTime:
		reply, meta = m.handleRescheduleTime(sess, intentMatch)
	case models.StateAppointmentCancelID:
		reply, meta = m.handleCancelID(sess, intentMatch)
	case models.StateAwaitingCancelConfirm:
		reply, meta = m.handleAwaitingCancelConfirm(sess, intentMatch)
	case models.StateAwaitingFollowup:
		reply, meta = m.handleFollowup(sess, text, intentMatch)
	default:
		sess.State = models.StateAwaitingPurpose
		reply, meta = m.routePurpose(sess, text, intentMatch)
	}
	sess.Append("assistant", reply)
	if meta == nil {
		meta = map[string]any{}
	}
	return &TurnResult{Message: reply, State: sess.State, Intent: intentMatch.Intent, Metadata: meta}
}

func (m *Manager) handleAwaitingName(sess *session.Session, text string) (string, map[string]any) {
	name := intent.ExtractName(text)
	in := intent.Detect(text)
	hasRequest := in.Intent != models.IntentUnknown &&
		in.Intent != models.IntentGreeting &&
		in.Intent != models.IntentAffirm &&
		in.Intent != models.IntentDeny

	// If we got a name AND the same utterance contained a real request,
	// honor both: save the name, then route the request immediately.
	if name != "" && hasRequest {
		sess.Slots["patient_name"] = name
		sess.State = models.StateAwaitingPurpose
		reply, meta := m.routePurpose(sess, text, in)
		if meta == nil {
			meta = map[string]any{}
		}
		meta["patient_name"] = name
		return fmt.Sprintf("Thank you, %s. %s", name, reply), meta
	}

	if name != "" {
		sess.Slots["patient_name"] = name
		sess.State = models.StateAwaitingPurpose
		return fmt.Sprintf(
			"Thank you, %s. How can I help you today? You can book, reschedule, "+
				"or cancel an appointment, or ask about our hours, doctors, or location.",
			name,
		), map[string]any{"patient_name": name}
	}

	// No name extracted, but caller has a clear request — accept it
	// and ask for the name later. Better UX than rejecting until a
	// name appears.
	if hasRequest {
		sess.State = models.StateAwaitingPurpose
		reply, meta := m.routePurpose(sess, text, in)
		return reply + " By the way, may I have your name for the record?", meta
	}

	// No name, no structured request — could be a free-form question
	// ("do you take Aetna?", "where do I park?"). Let the LLM fallback
	// answer it conversationally. If the LLM is unavailable, fall back
	// to the canned reprompt.
	//
	// After the LLM answers, transition to StateAwaitingFollowup so the
	// next turn doesn't try to extract a name from arbitrary text — the
	// caller has clearly chosen to ask questions before naming themselves.
	canned := "I didn't quite catch your name. Could you please tell me your full name?"
	reply := m.llmFallbackOr(sess, text, canned)
	if reply != canned {
		// LLM fallback fired — caller is in question mode now.
		sess.State = models.StateAwaitingFollowup
	}
	return reply, map[string]any{}
}

func (m *Manager) routePurpose(sess *session.Session, text string, in intent.Match) (string, map[string]any) {
	switch in.Intent {
	case models.IntentBookAppointment:
		return m.beginBooking(sess, in.Slots)
	case models.IntentRescheduleAppointment:
		return m.beginReschedule(sess, in.Slots)
	case models.IntentCancelAppointment:
		return m.beginCancel(sess, in.Slots)
	case models.IntentInquiryHours:
		sess.State = models.StateAwaitingFollowup
		cfg := config.Get()
		return fmt.Sprintf("Our hours are %s. Anything else?", cfg.ClinicHours),
			map[string]any{"hours": cfg.ClinicHours}
	case models.IntentInquiryLocation:
		sess.State = models.StateAwaitingFollowup
		cfg := config.Get()
		return fmt.Sprintf("We're located at %s. You can also reach us at %s. Anything else?",
				cfg.ClinicAddress, cfg.ClinicPhone),
			map[string]any{"address": cfg.ClinicAddress}
	case models.IntentInquiryDoctor:
		sess.State = models.StateAwaitingFollowup
		parts := []string{}
		for _, d := range m.apptSvc.AvailableDoctors() {
			parts = append(parts, fmt.Sprintf("%s (%s)", d.Name, d.Specialty))
		}
		return fmt.Sprintf("We currently have %s. Would you like to book with one of them?",
				strings.Join(parts, "; ")),
			map[string]any{"doctors": m.apptSvc.AvailableDoctors()}
	case models.IntentGoodbye:
		sess.State = models.StateEnded
		return fmt.Sprintf("Thank you for calling %s. Goodbye.", config.Get().ClinicName),
			map[string]any{}
	}
	canned := "I can help with booking, rescheduling, or cancelling an appointment, " +
		"or with general questions about our clinic. Which would you like to do?"
	reply := m.llmFallbackOr(sess, text, canned)
	if reply == canned {
		// LLM fallback didn't fire (no API key, or empty response) — track
		// how many times in a row we've returned this exact menu. After a
		// couple of misses, escalate instead of looping the same line, which
		// in real calls turns into 5+ identical replies and the caller
		// gives up. The Telephony "I'd like to talk to you" / "in the call"
		// utterances in the test transcript are exactly this case.
		miss, _ := sess.Slots["fallback_misses"].(int)
		miss++
		sess.Slots["fallback_misses"] = miss
		if miss >= 3 {
			delete(sess.Slots, "fallback_misses")
			cfg := config.Get()
			return fmt.Sprintf(
				"I'm having trouble understanding. A team member can call you back — "+
					"would you like to leave a callback number, or you can also reach us at %s.",
				cfg.ClinicPhone,
			), map[string]any{}
		}
		// Vary the prompt slightly on the second miss so it doesn't sound
		// like a parrot. Keeps the menu intent clear without word-for-word
		// repeat.
		if miss == 2 {
			reply = "Sorry, I missed that. I can help with general questions or with appointment changes; which would you prefer?"
		}
	} else {
		// Any non-canned reply means we got somewhere — reset the counter.
		delete(sess.Slots, "fallback_misses")
	}
	return reply, map[string]any{}
}

// --- Booking ---------------------------------------------------------

func (m *Manager) beginBooking(sess *session.Session, slots map[string]string) (string, map[string]any) {
	if v := slots["doctor"]; v != "" {
		sess.Slots["pending_doctor"] = v
	}
	if v := slots["date"]; v != "" {
		sess.Slots["pending_date"] = v
	}
	if v := slots["time"]; v != "" {
		sess.Slots["pending_time"] = v
	}
	if _, ok := sess.Slots["pending_doctor"]; !ok {
		sess.State = models.StateAppointmentBookDoctor
		return fmt.Sprintf(
			"I can help you book an appointment. Which doctor would you like to see? We have %s.",
			m.formatRoster(),
		), map[string]any{}
	}
	return m.askForTimeOrFinalize(sess, true)
}

func (m *Manager) handleBookDoctor(sess *session.Session, text string, in intent.Match) (string, map[string]any) {
	if in.Intent == models.IntentInquiryDoctor || looksLikeQuestion(text) {
		return fmt.Sprintf("We have %s. Which one would you like to see?", m.formatRoster()),
			map[string]any{}
	}
	if v := in.Slots["doctor"]; v != "" {
		sess.Slots["pending_doctor"] = v
	} else {
		cand := strings.TrimSpace(text)
		if len(strings.Fields(cand)) > 3 {
			canned := fmt.Sprintf(
				"I didn't catch which doctor. We have %s. Could you tell me just the doctor's name?",
				m.formatRoster(),
			)
			return m.llmFallbackOr(sess, text, canned), map[string]any{}
		}
		sess.Slots["pending_doctor"] = cand
	}
	return m.askForTimeOrFinalize(sess, true)
}

func (m *Manager) handleBookTime(sess *session.Session, in intent.Match) (string, map[string]any) {
	if v := in.Slots["date"]; v != "" {
		sess.Slots["pending_date"] = v
	}
	if v := in.Slots["time"]; v != "" {
		sess.Slots["pending_time"] = v
	}
	return m.askForTimeOrFinalize(sess, true)
}

func (m *Manager) askForTimeOrFinalize(sess *session.Session, booking bool) (string, map[string]any) {
	_, hasDate := sess.Slots["pending_date"]
	_, hasTime := sess.Slots["pending_time"]
	if !hasDate && !hasTime {
		if booking {
			sess.State = models.StateAppointmentBookTime
		} else {
			sess.State = models.StateAppointmentRescheduleTime
		}
		return "Sure. What day and time would work for you? ",
			map[string]any{}
	}
	return m.finalizeBooking(sess)
}

func (m *Manager) finalizeBooking(sess *session.Session) (string, map[string]any) {
	patientName, _ := sess.Slots["patient_name"].(string)
	if patientName == "" {
		patientName = "Caller"
	}
	doctor, _ := sess.Slots["pending_doctor"].(string)
	date, _ := sess.Slots["pending_date"].(string)
	tm, _ := sess.Slots["pending_time"].(string)

	appt, err := m.apptSvc.Book(patientName, doctor, date, tm)
	if err != nil {
		switch {
		case errIs(err, appointment.ErrUnknownDoctor):
			// Acknowledge the mismatch and offer a likely correction so
			// callers don't think the bot ignored them. "Hema" → "Did
			// you mean Dr. Emma?" instead of silently re-listing the roster.
			badGuess, _ := sess.Slots["pending_doctor"].(string)
			sess.State = models.StateAppointmentBookDoctor
			delete(sess.Slots, "pending_doctor")
			if suggest := m.apptSvc.SuggestDoctor(badGuess); suggest != "" && badGuess != "" {
				return fmt.Sprintf(
					"We don't have a Dr. %s on staff. Did you mean %s? Or pick from %s.",
					strings.TrimPrefix(strings.TrimPrefix(badGuess, "Dr. "), "Dr "),
					suggest, m.formatRoster(),
				), map[string]any{}
			}
			return fmt.Sprintf(
				"We have %s. Which one would you like to see?",
				m.formatRoster(),
			), map[string]any{}
		case errIs(err, appointment.ErrSlotUnavailable):
			sess.State = models.StateAppointmentBookTime
			delete(sess.Slots, "pending_date")
			delete(sess.Slots, "pending_time")
			return fmt.Sprintf("%v. Could you suggest another day or time?", err),
				map[string]any{}
		case errIs(err, appointment.ErrBadDate):
			// Caller said something we couldn't parse as a date (e.g. an
			// ambiguous phrasing the regex didn't catch). Re-prompt with a
			// concrete example so they can rephrase. Drop the slots so the
			// next turn re-runs extraction cleanly.
			sess.State = models.StateAppointmentBookTime
			delete(sess.Slots, "pending_date")
			delete(sess.Slots, "pending_time")
			return "I didn't catch that date. Could you say it like '27th May 2026' or 'next Tuesday'?",
				map[string]any{}
		case errIs(err, appointment.ErrBadTime):
			sess.State = models.StateAppointmentBookTime
			delete(sess.Slots, "pending_date")
			delete(sess.Slots, "pending_time")
			return "I didn't catch the time. Could you say it like '6 AM' or '3:30 PM'?",
				map[string]any{}
		default:
			sess.State = models.StateAppointmentBookTime
			delete(sess.Slots, "pending_date")
			delete(sess.Slots, "pending_time")
			return fmt.Sprintf(
				"I couldn't schedule that — %v. Please give me a clearer day and time, like 'Wednesday at 3 PM'.",
				err,
			), map[string]any{}
		}
	}

	delete(sess.Slots, "pending_doctor")
	delete(sess.Slots, "pending_date")
	delete(sess.Slots, "pending_time")
	sess.State = models.StateAwaitingFollowup
	// Mark the session as post-booking so the next "thank you" ends the
	// call cleanly (see the 1b shutoff in handleTurnInner).
	sess.Slots["booking_confirmed"] = true
	// Remember the last appointment so a follow-up "the time at 6 PM" /
	// "I want to change the time" knows which booking to amend without
	// asking for the confirmation number again.
	sess.Slots["last_appointment_id"] = appt.ID
	when := appt.ScheduledFor.Format("Monday, January 2 at 3:04 PM")
	return fmt.Sprintf(
			"You're booked with %s on %s. Your confirmation number is %s. Is there anything else I can help with?",
			appt.Doctor, when, appt.ID,
		), map[string]any{
			"appointment_id": appt.ID,
			"scheduled_for":  appt.ScheduledFor.Format("2006-01-02T15:04:05"),
		}
}

// --- Reschedule -------------------------------------------------------

func (m *Manager) beginReschedule(sess *session.Session, slots map[string]string) (string, map[string]any) {
	if v := slots["appointment_id"]; v != "" {
		sess.Slots["pending_appt_id"] = v
	}
	if v := slots["date"]; v != "" {
		sess.Slots["pending_date"] = v
	}
	if v := slots["time"]; v != "" {
		sess.Slots["pending_time"] = v
	}
	if _, ok := sess.Slots["pending_appt_id"]; !ok {
		sess.State = models.StateAppointmentRescheduleID
		return "Of course. What's your appointment confirmation number? It looks like 'APT-' followed by six characters.",
			map[string]any{}
	}
	return m.askForRescheduleTime(sess)
}

func (m *Manager) handleRescheduleID(sess *session.Session, in intent.Match) (string, map[string]any) {
	if v := in.Slots["appointment_id"]; v != "" {
		sess.Slots["pending_appt_id"] = v
		delete(sess.Slots, "reschedule_id_misses")
		return m.askForRescheduleTime(sess)
	}
	// Intent switches let the caller bail out without saying "thank you"
	// three times to escape. "Goodbye" / "book new" / "cancel" all jump
	// to the right flow instead of being treated as bad-id retries.
	if reply, ok := m.handleIDStateEscape(sess, in); ok {
		return reply, map[string]any{}
	}
	text := lastUserText(sess)
	if forgotPhraseRE.MatchString(text) {
		return m.handleForgotID(sess, "reschedule")
	}
	miss, _ := sess.Slots["reschedule_id_misses"].(int)
	miss++
	sess.Slots["reschedule_id_misses"] = miss
	if miss >= 2 {
		// After two misses, proactively offer the name-lookup escape so
		// the caller isn't trapped repeating "I forgot it".
		return m.handleForgotID(sess, "reschedule")
	}
	canned := "I didn't catch a valid confirmation number. It should look like 'APT-' followed by six letters or digits — or say 'I forgot it' and I can look it up by your name."
	return m.llmFallbackOr(sess, text, canned), map[string]any{}
}

// handleIDStateEscape: when a caller is being asked for an APT- number
// but instead expresses a different intent, switch flows. Returns ok=true
// when the caller's intent was handled here, ok=false to fall through
// to the canned re-prompt.
func (m *Manager) handleIDStateEscape(sess *session.Session, in intent.Match) (string, bool) {
	switch in.Intent {
	case models.IntentGoodbye:
		sess.State = models.StateEnded
		delete(sess.Slots, "reschedule_id_misses")
		delete(sess.Slots, "cancel_id_misses")
		return fmt.Sprintf("Thank you for calling %s. Goodbye.", config.Get().ClinicName), true
	case models.IntentBookAppointment:
		// Drop reschedule/cancel state, start a fresh booking.
		delete(sess.Slots, "pending_appt_id")
		delete(sess.Slots, "reschedule_id_misses")
		delete(sess.Slots, "cancel_id_misses")
		reply, _ := m.beginBooking(sess, in.Slots)
		return reply, true
	case models.IntentCancelAppointment:
		delete(sess.Slots, "pending_appt_id")
		delete(sess.Slots, "reschedule_id_misses")
		reply, _ := m.beginCancel(sess, in.Slots)
		return reply, true
	}
	return "", false
}

// forgotPhraseRE matches the common phrasings of "I don't have / can't
// remember the confirmation number". Kept narrow on purpose — false
// positives steal a re-prompt path the caller may need.
var forgotPhraseRE = regexp.MustCompile(`(?i)\b(?:forgot|forget|lost|don'?t\s+(?:have|remember|know)|can'?t\s+(?:find|remember))\b.*\b(?:it|number|confirmation|appointment|apt|id)\b|\bI\s+(?:forgot|forget)\s+it\b|\bdon'?t\s+have\s+the\s+number\b|\bdon'?t\s+remember\b`)

// handleForgotID looks up active appointments by patient name when the
// caller has lost their confirmation number. flow is "reschedule" or
// "cancel". With one match we proceed to the time prompt (or finalize
// cancel) directly; with multiple matches we list them so the caller
// can pick. With zero matches we apologize and offer to start over.
func (m *Manager) handleForgotID(sess *session.Session, flow string) (string, map[string]any) {
	patientName, _ := sess.Slots["patient_name"].(string)
	if strings.TrimSpace(patientName) == "" {
		// We never collected a name — can't look up. Fall back to the
		// canned prompt with a softened tone.
		return "I don't have your name on file to look it up. Could you give me your full name, please?",
			map[string]any{}
	}
	matches := m.apptSvc.FindByPatient(patientName)
	if len(matches) == 0 {
		sess.State = models.StateAwaitingPurpose
		return fmt.Sprintf(
			"I couldn't find any appointments under %s. Would you like to book a new one instead?",
			patientName,
		), map[string]any{}
	}
	if len(matches) == 1 {
		appt := matches[0]
		sess.Slots["pending_appt_id"] = appt.ID
		delete(sess.Slots, "reschedule_id_misses")
		delete(sess.Slots, "cancel_id_misses")
		when := appt.ScheduledFor.Format("Monday, January 2 at 3:04 PM")
		if flow == "cancel" {
			// Confirm before deleting — used to silently cancel which is
			// a security/UX miss (anyone with the caller's name can blow
			// away the booking). Stash the id and ask for yes/no.
			sess.State = models.StateAwaitingCancelConfirm
			return fmt.Sprintf(
				"I found %s with %s on %s, confirmation %s. Should I cancel it?",
				patientName, appt.Doctor, when, appt.ID,
			), map[string]any{"appointment_id": appt.ID}
		}
		// Reschedule flow — proceed to time prompt.
		ack := fmt.Sprintf(
			"Found it — %s with %s on %s, confirmation %s. ",
			patientName, appt.Doctor, when, appt.ID,
		)
		reply, meta := m.askForRescheduleTime(sess)
		return ack + reply, meta
	}
	// Multiple matches — list them and ask the caller to pick by index
	// or by speaking the doctor name. We stay in the same state; the
	// next turn's APT- regex or doctor-name match will resolve it.
	var lines []string
	for i, r := range matches {
		when := r.ScheduledFor.Format("Mon Jan 2 at 3:04 PM")
		lines = append(lines, fmt.Sprintf("%d) %s — %s with %s", i+1, r.ID, when, r.Doctor))
	}
	return fmt.Sprintf(
		"I found %d appointments under %s: %s. Could you read me the confirmation number of the one you mean?",
		len(matches), patientName, strings.Join(lines, "; "),
	), map[string]any{}
}

func (m *Manager) askForRescheduleTime(sess *session.Session) (string, map[string]any) {
	_, hasDate := sess.Slots["pending_date"]
	_, hasTime := sess.Slots["pending_time"]
	if hasDate || hasTime {
		return m.finalizeReschedule(sess)
	}
	sess.State = models.StateAppointmentRescheduleTime
	return "What new day and time would you like?", map[string]any{}
}

func (m *Manager) handleRescheduleTime(sess *session.Session, in intent.Match) (string, map[string]any) {
	if v := in.Slots["date"]; v != "" {
		sess.Slots["pending_date"] = v
	}
	if v := in.Slots["time"]; v != "" {
		sess.Slots["pending_time"] = v
	}
	return m.finalizeReschedule(sess)
}

func (m *Manager) finalizeReschedule(sess *session.Session) (string, map[string]any) {
	id, _ := sess.Slots["pending_appt_id"].(string)
	date, _ := sess.Slots["pending_date"].(string)
	tm, _ := sess.Slots["pending_time"].(string)

	appt, err := m.apptSvc.Reschedule(id, date, tm)
	if err != nil {
		switch {
		case errIs(err, appointment.ErrNotFound):
			sess.State = models.StateAppointmentRescheduleID
			delete(sess.Slots, "pending_appt_id")
			return fmt.Sprintf("%v. Could you double-check your confirmation number?", err),
				map[string]any{}
		case errIs(err, appointment.ErrSlotUnavailable):
			sess.State = models.StateAppointmentRescheduleTime
			delete(sess.Slots, "pending_date")
			delete(sess.Slots, "pending_time")
			return fmt.Sprintf("%v. Could you suggest another time?", err), map[string]any{}
		case errIs(err, appointment.ErrBadDate):
			sess.State = models.StateAppointmentRescheduleTime
			delete(sess.Slots, "pending_date")
			delete(sess.Slots, "pending_time")
			return "I didn't catch that date. Could you say it like '27th May 2026' or 'next Tuesday'?",
				map[string]any{}
		case errIs(err, appointment.ErrBadTime):
			sess.State = models.StateAppointmentRescheduleTime
			delete(sess.Slots, "pending_date")
			delete(sess.Slots, "pending_time")
			return "I didn't catch the time. Could you say it like '6 AM' or '3:30 PM'?",
				map[string]any{}
		default:
			sess.State = models.StateAppointmentRescheduleTime
			delete(sess.Slots, "pending_date")
			delete(sess.Slots, "pending_time")
			return fmt.Sprintf("I couldn't reschedule that — %v. What other time works?", err),
				map[string]any{}
		}
	}

	delete(sess.Slots, "pending_appt_id")
	delete(sess.Slots, "pending_date")
	delete(sess.Slots, "pending_time")
	sess.State = models.StateAwaitingFollowup
	// Same as finalizeBooking — keep last_appointment_id around so a
	// follow-up "the time at 6 PM" can amend without re-asking for the id.
	sess.Slots["last_appointment_id"] = appt.ID
	when := appt.ScheduledFor.Format("Monday, January 2 at 3:04 PM")
	return fmt.Sprintf(
			"All set — %s is now on %s with %s. Anything else I can help with?",
			appt.ID, when, appt.Doctor,
		), map[string]any{
			"appointment_id": appt.ID,
			"scheduled_for":  appt.ScheduledFor.Format("2006-01-02T15:04:05"),
		}
}

// --- Cancel ----------------------------------------------------------

func (m *Manager) beginCancel(sess *session.Session, slots map[string]string) (string, map[string]any) {
	if v := slots["appointment_id"]; v != "" {
		return m.finalizeCancel(sess, v)
	}
	sess.State = models.StateAppointmentCancelID
	return "I can help with that. What's your appointment confirmation number? It looks like 'APT-' followed by six characters.",
		map[string]any{}
}

func (m *Manager) handleCancelID(sess *session.Session, in intent.Match) (string, map[string]any) {
	if v := in.Slots["appointment_id"]; v != "" {
		delete(sess.Slots, "cancel_id_misses")
		return m.finalizeCancel(sess, v)
	}
	if reply, ok := m.handleIDStateEscape(sess, in); ok {
		return reply, map[string]any{}
	}
	text := lastUserText(sess)
	if forgotPhraseRE.MatchString(text) {
		return m.handleForgotID(sess, "cancel")
	}
	miss, _ := sess.Slots["cancel_id_misses"].(int)
	miss++
	sess.Slots["cancel_id_misses"] = miss
	if miss >= 2 {
		return m.handleForgotID(sess, "cancel")
	}
	return "I need a valid confirmation number to cancel — 'APT-' followed by six letters or digits — or say 'I forgot it' and I can look it up by your name.",
		map[string]any{}
}

func (m *Manager) finalizeCancel(sess *session.Session, id string) (string, map[string]any) {
	appt, err := m.apptSvc.Cancel(id)
	if err != nil {
		sess.State = models.StateAppointmentCancelID
		return fmt.Sprintf("%v. Could you read me the number again?", err), map[string]any{}
	}
	// Clear every per-appointment slot. Without this, a later "reschedule
	// the appointment" turn picked up the cancelled id from pending_appt_id
	// and the apptSvc rejected it with "no active appointment with that
	// id" instead of asking the caller for a fresh one.
	delete(sess.Slots, "pending_appt_id")
	delete(sess.Slots, "pending_date")
	delete(sess.Slots, "pending_time")
	delete(sess.Slots, "last_appointment_id")
	delete(sess.Slots, "cancel_id_misses")
	delete(sess.Slots, "reschedule_id_misses")
	sess.State = models.StateAwaitingFollowup
	return fmt.Sprintf(
			"Your appointment %s has been cancelled. Is there anything else I can help with?",
			appt.ID,
		), map[string]any{
			"appointment_id": appt.ID,
			"status":         appt.Status,
		}
}

// handleAwaitingCancelConfirm asks the caller to confirm a name-lookup
// cancel before we actually delete. Reached from handleForgotID's cancel
// branch when exactly one appointment matched.
func (m *Manager) handleAwaitingCancelConfirm(sess *session.Session, in intent.Match) (string, map[string]any) {
	id, _ := sess.Slots["pending_appt_id"].(string)
	switch in.Intent {
	case models.IntentAffirm:
		return m.finalizeCancel(sess, id)
	case models.IntentDeny, models.IntentGoodbye:
		delete(sess.Slots, "pending_appt_id")
		sess.State = models.StateAwaitingFollowup
		if in.Intent == models.IntentGoodbye {
			sess.State = models.StateEnded
			return fmt.Sprintf("Thank you for calling %s. Goodbye.", config.Get().ClinicName),
				map[string]any{}
		}
		return "OK, I won't cancel it. Is there anything else I can help with?",
			map[string]any{}
	}
	// Anything else — re-prompt narrowly. Don't fall through to the generic
	// menu; the caller is one step from a destructive action.
	return fmt.Sprintf("Should I cancel appointment %s? Please say yes or no.", id),
		map[string]any{}
}

// --- Followup --------------------------------------------------------

func (m *Manager) handleFollowup(sess *session.Session, text string, in intent.Match) (string, map[string]any) {
	clinicName := config.Get().ClinicName
	// Explicit goodbye — caller is done, end the call cleanly.
	if in.Intent == models.IntentGoodbye {
		sess.State = models.StateEnded
		return fmt.Sprintf("Thank you for calling %s. Goodbye.", clinicName),
			map[string]any{}
	}
	// Plain "no" / "not really" — don't hang up immediately; let the LLM
	// soften with an offer to help with anything else, since the caller
	// declined the LAST suggestion, not the whole call. If the LLM is
	// unavailable, fall back to the legacy behavior (end the call).
	if in.Intent == models.IntentDeny {
		canned := fmt.Sprintf("Thank you for calling %s. Goodbye.", clinicName)
		reply := m.llmFallbackOr(sess, text, canned)
		if reply == canned {
			sess.State = models.StateEnded
		}
		// If the LLM produced something else, keep state at follow-up so
		// the caller can keep talking — don't end mid-thought.
		return reply, map[string]any{}
	}
	// Bare time/date utterance after a booking — caller is fixing the slot
	// they just booked (e.g. "the time at 7 PM" right after the bot
	// confirmed 7 AM). Treat as an implicit reschedule of the last
	// appointment instead of falling through to the generic menu.
	if in.Intent == models.IntentUnknown {
		_, hasTime := in.Slots["time"]
		_, hasDate := in.Slots["date"]
		if hasTime || hasDate {
			if lastID, _ := sess.Slots["last_appointment_id"].(string); lastID != "" {
				sess.Slots["pending_appt_id"] = lastID
				if v := in.Slots["date"]; v != "" {
					sess.Slots["pending_date"] = v
				}
				if v := in.Slots["time"]; v != "" {
					sess.Slots["pending_time"] = v
				}
				return m.finalizeReschedule(sess)
			}
		}
	}
	sess.State = models.StateAwaitingPurpose
	return m.routePurpose(sess, text, in)
}

// --- helpers ---------------------------------------------------------

func (m *Manager) formatRoster() string {
	doctors := m.apptSvc.AvailableDoctors()
	parts := make([]string, len(doctors))
	for i, d := range doctors {
		parts[i] = d.Name
	}
	return strings.Join(parts, ", ")
}

func looksLikeQuestion(text string) bool {
	if strings.Contains(text, "?") {
		return true
	}
	return questionRE.MatchString(text)
}

func errIs(err, target error) bool {
	return errors.Is(err, target)
}
