// Package llm wraps the Anthropic Go SDK for the receptionist's
// conversational turns.
//
// Design notes:
//   - Emergency detection runs in conversation_manager BEFORE this module
//     is called. We never let the model be the safety arbiter for plain
//     emergency keywords.
//   - System prompt + tool definitions are stable across sessions, so we
//     mark cache_control: ephemeral on the system block. After the first
//     turn of any call, prefix tokens cost ~10% of base.
//   - Manual agentic loop: each caller utterance is one HTTP request that
//     may require multiple Claude turns (call → tool_use → result → reply).
//     We loop until stop_reason == "end_turn" with a small iteration cap.
package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	"github.com/globussoft/callified-backend/internal/receptionist/ambulance"
	"github.com/globussoft/callified-backend/internal/receptionist/appointment"
	"github.com/globussoft/callified-backend/internal/receptionist/config"
	"github.com/globussoft/callified-backend/internal/receptionist/session"
)

const maxToolIterations = 6

// Agent is the LLM-backed conversational layer.
type Agent struct {
	client      *anthropic.Client
	enabled     bool
	model       string
	maxTokens   int
	apptSvc     *appointment.Service
	ambulanceSv *ambulance.Service
	systemBlock string
	tools       []anthropic.ToolUnionParam
}

// New wires up the agent. Returns one with Enabled()=false if no API key
// is configured — callers fall back to the rule-based path in that case.
func New(apptSvc *appointment.Service, ambSvc *ambulance.Service) *Agent {
	cfg := config.Get()
	a := &Agent{
		model:       cfg.LLMModel,
		maxTokens:   cfg.LLMMaxTokens,
		apptSvc:     apptSvc,
		ambulanceSv: ambSvc,
	}
	if cfg.AnthropicAPIKey == "" {
		return a
	}
	c := anthropic.NewClient(option.WithAPIKey(cfg.AnthropicAPIKey))
	a.client = &c
	a.enabled = true
	a.systemBlock = a.buildSystemPrompt()
	a.tools = a.buildTools()
	log.Printf("Anthropic client initialized (model=%s)", cfg.LLMModel)
	return a
}

// Enabled reports whether the LLM path should be used.
func (a *Agent) Enabled() bool { return a.enabled }

// --- System prompt + tools -------------------------------------------

func (a *Agent) buildSystemPrompt() string {
	cfg := config.Get()
	return fmt.Sprintf(`You are an AI receptionist for %s, an American medical clinic. You answer phone calls from patients in English. Speak naturally and conversationally — this is a phone call, so keep replies short, easy to listen to, and free of lists, markdown, or technical jargon.

Clinic information:
- Name: %s
- Hours: %s
- Address: %s
- Phone: %s

Your duties:
1. Greet warmly, ask for the caller's name and how you can help.
2. Book, reschedule, and cancel appointments using the tools.
3. Answer questions about hours, location, and available doctors.
4. Ask for one piece of information at a time — never overload the caller.

Important rules:
- Medical emergencies (chest pain, difficulty breathing, unconsciousness, severe bleeding, suicidal thoughts, etc.) are handled automatically by the system before your turn — you will not see them.
- Confirmation numbers look like 'APT-XXXXXX'. Read them out clearly: say each character separately so the caller can write them down.
- Accept natural-language dates and times — 'tomorrow at 10 AM', 'Friday afternoon', etc. The book_appointment tool will parse them.
- Always confirm successful bookings with the date, time, doctor, and confirmation number.
- If a tool returns an error, apologize briefly, explain what went wrong in plain language, and offer the next step.
- Do not invent doctors, hours, or services — call lookup_doctor_availability if asked.
- When the caller says goodbye or has no more questions, call end_call after delivering a brief sign-off.`,
		cfg.ClinicName, cfg.ClinicName, cfg.ClinicHours,
		cfg.ClinicAddress, cfg.ClinicPhone)
}

func (a *Agent) buildTools() []anthropic.ToolUnionParam {
	specs := []anthropic.ToolParam{
		{
			Name:        "book_appointment",
			Description: anthropic.String("Book a new appointment for the caller. Use after you have the patient's name, the doctor (or specialty), the date, and the time."),
			InputSchema: anthropic.ToolInputSchemaParam{
				Properties: map[string]any{
					"patient_name": map[string]any{
						"type":        "string",
						"description": "Full name of the patient.",
					},
					"doctor": map[string]any{
						"type":        "string",
						"description": "Doctor's last name (e.g. 'Patel') or specialty (e.g. 'pediatrician').",
					},
					"date": map[string]any{
						"type":        "string",
						"description": "Natural-language date — 'tomorrow', 'Monday', '2026-05-10', '5/10', etc.",
					},
					"time": map[string]any{
						"type":        "string",
						"description": "Natural-language time — '10 AM', '2:30 PM', '14:00'.",
					},
				},
				Required: []string{"patient_name", "doctor", "date", "time"},
			},
		},
		{
			Name:        "reschedule_appointment",
			Description: anthropic.String("Move an existing appointment to a new date/time. Requires the caller's confirmation number."),
			InputSchema: anthropic.ToolInputSchemaParam{
				Properties: map[string]any{
					"appointment_id": map[string]any{
						"type":        "string",
						"description": "Confirmation number, format 'APT-XXXXXX'.",
					},
					"date": map[string]any{"type": "string"},
					"time": map[string]any{"type": "string"},
				},
				Required: []string{"appointment_id", "date", "time"},
			},
		},
		{
			Name:        "cancel_appointment",
			Description: anthropic.String("Cancel an existing appointment by confirmation number."),
			InputSchema: anthropic.ToolInputSchemaParam{
				Properties: map[string]any{
					"appointment_id": map[string]any{
						"type":        "string",
						"description": "Confirmation number, format 'APT-XXXXXX'.",
					},
				},
				Required: []string{"appointment_id"},
			},
		},
		{
			Name:        "lookup_doctor_availability",
			Description: anthropic.String("Return the list of doctors at the clinic and their specialties. Use when the caller asks who is available or which doctor handles a condition."),
			InputSchema: anthropic.ToolInputSchemaParam{
				Properties: map[string]any{},
			},
		},
		{
			Name:        "end_call",
			Description: anthropic.String("Signal that the call is ending. Call this AFTER you have said goodbye, when the caller has confirmed they have no more questions."),
			InputSchema: anthropic.ToolInputSchemaParam{
				Properties: map[string]any{
					"reason": map[string]any{
						"type":        "string",
						"description": "Brief reason — e.g. 'caller said goodbye'.",
					},
				},
			},
		},
	}
	tools := make([]anthropic.ToolUnionParam, len(specs))
	for i := range specs {
		t := specs[i]
		tools[i] = anthropic.ToolUnionParam{OfTool: &t}
	}
	return tools
}

// --- Public API ------------------------------------------------------

// Result is the outcome of one Respond() call.
type Result struct {
	Reply    string
	Metadata map[string]any
	EndCall  bool
}

// Respond drives one caller turn through Claude.
//
// Assumes the session transcript already contains the new user message
// (the conversation_manager appends it before calling this).
func (a *Agent) Respond(sess *session.Session) Result {
	if !a.enabled {
		return Result{Reply: "(LLM not configured)", Metadata: map[string]any{}}
	}

	// Build messages from the transcript. Claude requires alternating
	// user/assistant turns starting with 'user' — drop any leading
	// assistant turn (the opening greeting from start_call).
	messages := []anthropic.MessageParam{}
	for _, t := range sess.Transcript {
		switch t.Role {
		case "user":
			messages = append(messages, anthropic.NewUserMessage(anthropic.NewTextBlock(t.Text)))
		case "assistant":
			if len(messages) == 0 {
				continue // drop leading assistant turn
			}
			messages = append(messages, anthropic.NewAssistantMessage(anthropic.NewTextBlock(t.Text)))
		}
	}
	if len(messages) == 0 {
		return Result{Reply: "I'm sorry, could you repeat that?", Metadata: map[string]any{}}
	}

	system := []anthropic.TextBlockParam{
		{
			Text:         a.systemBlock,
			CacheControl: anthropic.NewCacheControlEphemeralParam(),
		},
	}

	metadata := map[string]any{}
	endCall := false

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	for i := 0; i < maxToolIterations; i++ {
		resp, err := a.client.Messages.New(ctx, anthropic.MessageNewParams{
			Model:     anthropic.Model(a.model),
			MaxTokens: int64(a.maxTokens),
			System:    system,
			Tools:     a.tools,
			Messages:  messages,
		})
		if err != nil {
			log.Printf("anthropic call failed: %v", err)
			return Result{
				Reply:    "I'm sorry, I'm having a technical issue. Could you try calling back in a moment?",
				Metadata: map[string]any{"error": "llm_api_error"},
			}
		}

		if resp.StopReason == anthropic.StopReasonEndTurn {
			text := firstText(resp)
			if text == "" {
				text = "Is there anything else I can help with?"
			}
			return Result{Reply: text, Metadata: metadata, EndCall: endCall}
		}

		if resp.StopReason == anthropic.StopReasonToolUse {
			messages = append(messages, resp.ToParam())
			toolResults := []anthropic.ContentBlockParamUnion{}
			for _, block := range resp.Content {
				use, ok := block.AsAny().(anthropic.ToolUseBlock)
				if !ok {
					continue
				}
				resultText, meta, ec := a.executeTool(use.Name, use.JSON.Input.Raw(), sess)
				for k, v := range meta {
					metadata[k] = v
				}
				if ec {
					endCall = true
				}
				toolResults = append(toolResults,
					anthropic.NewToolResultBlock(use.ID, resultText, false))
			}
			if len(toolResults) == 0 {
				break
			}
			messages = append(messages, anthropic.NewUserMessage(toolResults...))
			continue
		}

		// Unhandled stop reasons — bail with whatever text we have.
		text := firstText(resp)
		if text == "" {
			text = "I'm sorry, could you say that again?"
		}
		return Result{Reply: text, Metadata: metadata, EndCall: endCall}
	}

	log.Printf("tool-use loop hit iteration cap (session=%s)", sess.ID)
	return Result{
		Reply:    "I'm having trouble completing that. Could you try again in a moment?",
		Metadata: metadata,
	}
}

// --- Tool execution --------------------------------------------------

func (a *Agent) executeTool(name, rawInput string, sess *session.Session) (string, map[string]any, bool) {
	switch name {
	case "book_appointment":
		var in struct {
			PatientName string `json:"patient_name"`
			Doctor      string `json:"doctor"`
			Date        string `json:"date"`
			Time        string `json:"time"`
		}
		if err := json.Unmarshal([]byte(rawInput), &in); err != nil {
			return "Error: malformed input.", map[string]any{"error": "bad_input"}, false
		}
		appt, err := a.apptSvc.Book(in.PatientName, in.Doctor, in.Date, in.Time)
		if err != nil {
			if errors.Is(err, appointment.ErrUnknownDoctor) {
				roster := formatRoster(a.apptSvc)
				return fmt.Sprintf("Error: %v. Available doctors: %s.", err, roster),
					map[string]any{"error": "unknown_doctor"}, false
			}
			if errors.Is(err, appointment.ErrSlotUnavailable) {
				return fmt.Sprintf("Error: %v", err), map[string]any{"error": "slot_unavailable"}, false
			}
			return fmt.Sprintf("Error: %v", err), map[string]any{"error": err.Error()}, false
		}
		sess.Slots["patient_name"] = in.PatientName
		when := appt.ScheduledFor.Format("Monday, January 2 at 3:04 PM")
		return fmt.Sprintf("Booked. Confirmation %s. Doctor: %s. When: %s.",
				appt.ID, appt.Doctor, when),
			map[string]any{
				"appointment_id": appt.ID,
				"doctor":         appt.Doctor,
				"scheduled_for":  appt.ScheduledFor.Format(time.RFC3339),
			}, false

	case "reschedule_appointment":
		var in struct {
			AppointmentID string `json:"appointment_id"`
			Date          string `json:"date"`
			Time          string `json:"time"`
		}
		json.Unmarshal([]byte(rawInput), &in)
		appt, err := a.apptSvc.Reschedule(in.AppointmentID, in.Date, in.Time)
		if err != nil {
			return fmt.Sprintf("Error: %v", err), map[string]any{"error": err.Error()}, false
		}
		when := appt.ScheduledFor.Format("Monday, January 2 at 3:04 PM")
		return fmt.Sprintf("Rescheduled. %s now %s with %s.", appt.ID, when, appt.Doctor),
			map[string]any{
				"appointment_id": appt.ID,
				"doctor":         appt.Doctor,
				"scheduled_for":  appt.ScheduledFor.Format(time.RFC3339),
			}, false

	case "cancel_appointment":
		var in struct {
			AppointmentID string `json:"appointment_id"`
		}
		json.Unmarshal([]byte(rawInput), &in)
		appt, err := a.apptSvc.Cancel(in.AppointmentID)
		if err != nil {
			return fmt.Sprintf("Error: %v", err), map[string]any{"error": err.Error()}, false
		}
		return fmt.Sprintf("Cancelled. %s status now %s.", appt.ID, appt.Status),
			map[string]any{"appointment_id": appt.ID, "status": appt.Status}, false

	case "lookup_doctor_availability":
		roster := a.apptSvc.AvailableDoctors()
		b, _ := json.Marshal(roster)
		return string(b), map[string]any{"doctors": roster}, false

	case "end_call":
		var in struct {
			Reason string `json:"reason"`
		}
		json.Unmarshal([]byte(rawInput), &in)
		return "Call ended.", map[string]any{"reason": in.Reason}, true

	default:
		return fmt.Sprintf("Unknown tool: %s", name), map[string]any{"error": "unknown_tool"}, false
	}
}

func firstText(m *anthropic.Message) string {
	for _, b := range m.Content {
		if t, ok := b.AsAny().(anthropic.TextBlock); ok {
			return t.Text
		}
	}
	return ""
}

func formatRoster(svc *appointment.Service) string {
	doctors := svc.AvailableDoctors()
	names := make([]string, len(doctors))
	for i, d := range doctors {
		names[i] = d.Name
	}
	return joinComma(names)
}

func joinComma(parts []string) string {
	out := ""
	for i, p := range parts {
		if i > 0 {
			out += ", "
		}
		out += p
	}
	return out
}

// staticInit guards the package against being imported before config is loaded.
var staticInit sync.Once
