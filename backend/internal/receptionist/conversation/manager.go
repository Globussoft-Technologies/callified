// Package conversation orchestrates one caller's dialog: emergency check,
// LLM dispatch (when configured), and the rule-based fallback FSM.
//
// The most important invariant: emergency keyword detection runs on
// EVERY utterance regardless of state and short-circuits both the LLM
// and the FSM. Never let the model decide whether "chest pain" is an
// emergency.
package conversation

import (
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
	store    *session.Store
	apptSvc  *appointment.Service
	ambSvc   *ambulance.Service
	llmAgent *llm.Agent
}

// New constructs a Manager.
func New(store *session.Store, apptSvc *appointment.Service,
	ambSvc *ambulance.Service, llmAgent *llm.Agent) *Manager {
	return &Manager{store: store, apptSvc: apptSvc, ambSvc: ambSvc, llmAgent: llmAgent}
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

	return "I didn't quite catch your name. Could you please tell me your full name?",
		map[string]any{}
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
	return "I can help with booking, rescheduling, or cancelling an appointment, " +
			"or with general questions about our clinic. Which would you like to do?",
		map[string]any{}
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
			return fmt.Sprintf(
				"I didn't catch which doctor. We have %s. Could you tell me just the doctor's name?",
				m.formatRoster(),
			), map[string]any{}
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
		return "Sure. What day and time would work for you? For example, 'tomorrow at 10 AM' or 'Friday at 2 PM'.",
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
			sess.State = models.StateAppointmentBookDoctor
			delete(sess.Slots, "pending_doctor")
			return fmt.Sprintf(
				"I'm sorry — %v. We have %s. Which one would you like to see?",
				err, m.formatRoster(),
			), map[string]any{}
		case errIs(err, appointment.ErrSlotUnavailable):
			sess.State = models.StateAppointmentBookTime
			delete(sess.Slots, "pending_date")
			delete(sess.Slots, "pending_time")
			return fmt.Sprintf("%v. Could you suggest another day or time?", err),
				map[string]any{}
		default:
			sess.State = models.StateAppointmentBookTime
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
		return m.askForRescheduleTime(sess)
	}
	return "I didn't catch a valid confirmation number. It should look like 'APT-' followed by six letters or digits.",
		map[string]any{}
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
		default:
			sess.State = models.StateAppointmentRescheduleTime
			return fmt.Sprintf("I couldn't reschedule that — %v. What other time works?", err),
				map[string]any{}
		}
	}

	delete(sess.Slots, "pending_appt_id")
	delete(sess.Slots, "pending_date")
	delete(sess.Slots, "pending_time")
	sess.State = models.StateAwaitingFollowup
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
		return m.finalizeCancel(sess, v)
	}
	return "I need a valid confirmation number to cancel — it should look like 'APT-' followed by six letters or digits.",
		map[string]any{}
}

func (m *Manager) finalizeCancel(sess *session.Session, id string) (string, map[string]any) {
	appt, err := m.apptSvc.Cancel(id)
	if err != nil {
		sess.State = models.StateAppointmentCancelID
		return fmt.Sprintf("%v. Could you read me the number again?", err), map[string]any{}
	}
	sess.State = models.StateAwaitingFollowup
	return fmt.Sprintf(
			"Your appointment %s has been cancelled. Is there anything else I can help with?",
			appt.ID,
		), map[string]any{
			"appointment_id": appt.ID,
			"status":         appt.Status,
		}
}

// --- Followup --------------------------------------------------------

func (m *Manager) handleFollowup(sess *session.Session, text string, in intent.Match) (string, map[string]any) {
	if in.Intent == models.IntentGoodbye || in.Intent == models.IntentDeny {
		sess.State = models.StateEnded
		return fmt.Sprintf("Thank you for calling %s. Goodbye.", config.Get().ClinicName),
			map[string]any{}
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
