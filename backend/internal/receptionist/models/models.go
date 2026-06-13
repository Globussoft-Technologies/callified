// Package models holds the request/response types and FSM state enums
// shared across HTTP handlers and the conversation engine.
package models

import "time"

// ConversationState is the finite-state-machine state for a caller.
type ConversationState string

const (
	StateGreeting                  ConversationState = "greeting"
	StateAwaitingName              ConversationState = "awaiting_name"
	StateAwaitingPurpose           ConversationState = "awaiting_purpose"
	StateAppointmentBookDoctor     ConversationState = "appointment_book_doctor"
	StateAppointmentBookTime       ConversationState = "appointment_book_time"
	StateAppointmentRescheduleID   ConversationState = "appointment_reschedule_id"
	StateAppointmentRescheduleTime ConversationState = "appointment_reschedule_time"
	StateAppointmentCancelID       ConversationState = "appointment_cancel_id"
	StateAwaitingFollowup          ConversationState = "awaiting_followup"
	StateEmergency                 ConversationState = "emergency"
	StateEnded                     ConversationState = "ended"
)

// IntentType is what the rule-based intent detector classifies an
// utterance as. The LLM path doesn't use these.
type IntentType string

const (
	IntentGreeting              IntentType = "greeting"
	IntentProvideName           IntentType = "provide_name"
	IntentBookAppointment       IntentType = "book_appointment"
	IntentRescheduleAppointment IntentType = "reschedule_appointment"
	IntentCancelAppointment     IntentType = "cancel_appointment"
	IntentInquiryHours          IntentType = "inquiry_hours"
	IntentInquiryLocation       IntentType = "inquiry_location"
	IntentInquiryDoctor         IntentType = "inquiry_doctor"
	IntentEmergency             IntentType = "emergency"
	IntentAffirm                IntentType = "affirm"
	IntentDeny                  IntentType = "deny"
	IntentGoodbye               IntentType = "goodbye"
	IntentUnknown               IntentType = "unknown"
)

// StartCallRequest opens a new caller session.
type StartCallRequest struct {
	CallerID  string `json:"caller_id,omitempty"`
	Language  string `json:"language,omitempty"`
	AgentName string `json:"agent_name,omitempty"` // optional — replaces "the AI receptionist" in the greeting
}

// StartCallResponse returns the session id and opening greeting.
type StartCallResponse struct {
	SessionID string            `json:"session_id"`
	Message   string            `json:"message"`
	State     ConversationState `json:"state"`
}

// ProcessInputRequest sends one caller utterance.
type ProcessInputRequest struct {
	SessionID string `json:"session_id"`
	Text      string `json:"text"`
}

// ProcessInputResponse is one assistant reply.
type ProcessInputResponse struct {
	SessionID   string            `json:"session_id"`
	Message     string            `json:"message"`
	State       ConversationState `json:"state"`
	Intent      IntentType        `json:"intent"`
	IsEmergency bool              `json:"is_emergency"`
	Metadata    map[string]any    `json:"metadata"`
}

// EndCallRequest closes the session.
type EndCallRequest struct {
	SessionID string `json:"session_id"`
}

// EndCallResponse returns the full transcript.
type EndCallResponse struct {
	SessionID  string           `json:"session_id"`
	Message    string           `json:"message"`
	Transcript []map[string]any `json:"transcript"`
}

// Appointment is the public booking record returned by the API.
type Appointment struct {
	ID           string    `json:"id"`
	PatientName  string    `json:"patient_name"`
	Doctor       string    `json:"doctor"`
	ScheduledFor time.Time `json:"scheduled_for"`
	Status       string    `json:"status"`
	CreatedAt    time.Time `json:"created_at"`
}
