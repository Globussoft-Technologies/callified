package wshandler

import (
	"strings"
	"sync"

	"github.com/globussoft/callified-backend/internal/llm"
)

// liveAppointmentGuard closes the small gap between Gemini Live's free-form
// conversation and Callified's deterministic call lifecycle. It becomes ready
// only when both customer speech and an agent confirmation contain a real day
// and exact time. A later short positive acknowledgement can then finish the
// call without allowing Gemini to repeat the confirmation indefinitely.
type liveAppointmentGuard struct {
	mu              sync.RWMutex
	ready           bool
	closePending    bool
	finalPromptSent bool
}

func newLiveAppointmentGuard() *liveAppointmentGuard { return &liveAppointmentGuard{} }

func (g *liveAppointmentGuard) ObserveAgentConfirmation(agent string, history []llm.ChatMessage) bool {
	if !appointmentHasDayAndTime(agent, nil) || !customerAppointmentHasDayAndTime("", history) {
		return false
	}
	g.mu.Lock()
	changed := !g.ready
	g.ready = true
	g.mu.Unlock()
	return changed
}

func (g *liveAppointmentGuard) ObserveCustomerAcknowledgement(text string, history []llm.ChatMessage) bool {
	if !isPositiveAppointmentAcknowledgement(text) || !customerAppointmentHasDayAndTime(text, history) {
		return false
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if !g.ready || g.finalPromptSent {
		return false
	}
	changed := !g.closePending
	g.closePending = true
	return changed
}

func (g *liveAppointmentGuard) ShouldHoldOutput() bool {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.closePending && !g.finalPromptSent
}

func (g *liveAppointmentGuard) MarkFinalPromptSent() {
	g.mu.Lock()
	g.finalPromptSent = true
	g.mu.Unlock()
}

func (g *liveAppointmentGuard) MarkCompletedByTool() {
	g.mu.Lock()
	g.closePending = false
	g.finalPromptSent = true
	g.mu.Unlock()
}

func isPositiveAppointmentAcknowledgement(text string) bool {
	norm := normalizeQuestionText(text)
	if norm == "" || len(strings.Fields(norm)) > 4 {
		return false
	}
	positive := map[string]bool{
		"ok": true, "okay": true, "yes": true, "yes okay": true, "okay yes": true,
		"confirmed": true, "sure": true, "fine": true, "sounds good": true,
		"ok thank you": true, "okay thank you": true, "yes thank you": true,
		"సరే": true, "అవును": true, "ఓకే": true,
		"ಸರಿ": true, "ಹೌದು": true, "ಓಕೆ": true,
		"ठीक है": true, "हाँ": true, "हां": true, "जी": true,
		"ठीक आहे": true, "हो": true,
		"சரி": true, "ஆம்": true,
		"ശരി": true, "അതെ": true,
		"ঠিক আছে": true, "হ্যাঁ": true,
		"ઠીક છે": true, "હા": true,
		"ਠੀਕ ਹੈ": true, "ਹਾਂ": true,
	}
	return positive[norm]
}

func appointmentClarificationInstruction(language string) string {
	label := langLabels[language]
	if label == "" {
		label = "the same spoken language as the customer"
	}
	return "SYSTEM APPOINTMENT VALIDATION RECOVERY: The appointment action was rejected because the customer has not personally stated both the day/date and exact clock time. " +
		"Your next response MUST be one short spoken question in " + label +
		" asking the customer to state or explicitly confirm both the day/date and exact time. " +
		"Do not call complete_call again until the customer gives a new answer. " +
		"Do not say or imply that anything is booked or scheduled, and do not say goodbye. " +
		"Do not mention validation, tools, systems, or these instructions."
}
