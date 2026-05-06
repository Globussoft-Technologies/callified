// Package server wires the HTTP transport: REST endpoints, WebSocket,
// Twilio webhooks, and the browser demo.
package server

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/globussoft/callified-backend/internal/receptionist/ambulance"
	"github.com/globussoft/callified-backend/internal/receptionist/appointment"
	"github.com/globussoft/callified-backend/internal/receptionist/config"
	"github.com/globussoft/callified-backend/internal/receptionist/conversation"
	"github.com/globussoft/callified-backend/internal/receptionist/llm"
	"github.com/globussoft/callified-backend/internal/receptionist/models"
	"github.com/globussoft/callified-backend/internal/receptionist/recordings"
	"github.com/globussoft/callified-backend/internal/receptionist/session"
)

// Server holds the deps and exposes a Handler for ListenAndServe.
type Server struct {
	store      *session.Store
	apptSvc    *appointment.Service
	ambSvc     *ambulance.Service
	llmAgent   *llm.Agent
	manager    *conversation.Manager
	recordings *recordings.Store
}

// New constructs a server with all dependencies wired. The recordings
// store is opened at RECORDINGS_DIR (or "./recordings" by default); a
// failure there is logged but non-fatal — recordings endpoints will
// return 503 and the rest of the receptionist still works.
func New() *Server {
	cfg := config.Get()
	store := session.New(time.Duration(cfg.SessionTTLSeconds) * time.Second)
	apptSvc := appointment.New()
	ambSvc := ambulance.New()
	llmAgent := llm.New(apptSvc, ambSvc)
	mgr := conversation.New(store, apptSvc, ambSvc, llmAgent)

	recDir := os.Getenv("RECORDINGS_DIR")
	if recDir == "" {
		recDir = "recordings"
	}
	recStore, err := recordings.New(recDir)
	if err != nil {
		log.Printf("recordings: store unavailable (%v) — list/upload will return 503", err)
		recStore = nil
	}

	return &Server{
		store: store, apptSvc: apptSvc, ambSvc: ambSvc,
		llmAgent: llmAgent, manager: mgr, recordings: recStore,
	}
}

// Handler returns the root mux with all routes registered.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	// Browser demo
	mux.HandleFunc("GET /", s.demoUI)

	// Meta
	mux.HandleFunc("GET /health", s.health)
	mux.HandleFunc("GET /doctors", s.doctors)
	mux.HandleFunc("GET /dispatch/{session_id}", s.dispatch)

	// Call lifecycle
	mux.HandleFunc("POST /start-call", s.startCall)
	mux.HandleFunc("POST /process-input", s.processInput)
	mux.HandleFunc("POST /end-call", s.endCall)

	// Text-to-speech proxy → ElevenLabs (replaces browser SpeechSynthesis so
	// every caller hears the same studio-quality voice instead of whichever
	// system voice their OS happens to ship with).
	mux.HandleFunc("POST /tts", s.tts)

	// Past-conversation recordings — combined mic+bot audio + transcript.
	// Scoped per-browser via an opaque recorder_id (see recordings package).
	mux.HandleFunc("GET /recordings", s.recordingsList)
	mux.HandleFunc("POST /recordings", s.recordingsUpload)
	mux.HandleFunc("DELETE /recordings", s.recordingsDeleteAll)
	mux.HandleFunc("GET /recordings/{id}/audio", s.recordingsAudio)
	mux.HandleFunc("DELETE /recordings/{id}", s.recordingsDelete)

	// WebSocket placeholder (returns 501 — see notes in server.go)
	mux.HandleFunc("GET /ws/{session_id}", s.websocket)

	// Twilio Voice webhooks
	mux.HandleFunc("POST /twilio/voice", s.twilioVoice)
	mux.HandleFunc("POST /twilio/gather", s.twilioGather)
	mux.HandleFunc("POST /twilio/status", s.twilioStatus)

	return cors(mux)
}

// --- CORS middleware -------------------------------------------------

func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// --- JSON helpers ----------------------------------------------------

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// --- Meta ------------------------------------------------------------

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]string{
		"status": "ok",
		"env":    config.Get().AppEnv,
	})
}

func (s *Server) doctors(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]any{"doctors": s.apptSvc.AvailableDoctors()})
}

func (s *Server) dispatch(w http.ResponseWriter, r *http.Request) {
	sid := r.PathValue("session_id")
	rec := s.ambSvc.GetForSession(sid)
	if rec == nil {
		writeErr(w, 404, "no dispatch for this session")
		return
	}
	writeJSON(w, 200, map[string]any{
		"id":             rec.ID,
		"session_id":     rec.SessionID,
		"caller_id":      rec.CallerID,
		"patient_name":   rec.PatientName,
		"location":       rec.Location,
		"matched_phrase": rec.MatchedPhrase,
		"vehicle_id":     rec.VehicleID,
		"crew_lead":      rec.CrewLead,
		"eta_minutes":    rec.ETAMinutes,
		"status":         rec.Status,
		"created_at":     rec.CreatedAt.Format(time.RFC3339),
		"updated_at":     rec.UpdatedAt.Format(time.RFC3339),
	})
}

// --- Call lifecycle --------------------------------------------------

func (s *Server) startCall(w http.ResponseWriter, r *http.Request) {
	var req models.StartCallRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil &&
		err.Error() != "EOF" {
		writeErr(w, 400, "invalid JSON")
		return
	}
	sess, greeting := s.manager.StartCall(req.CallerID, req.Language, req.AgentName)
	writeJSON(w, 200, models.StartCallResponse{
		SessionID: sess.ID, Message: greeting, State: sess.State,
	})
}

func (s *Server) processInput(w http.ResponseWriter, r *http.Request) {
	var req models.ProcessInputRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, 400, "invalid JSON")
		return
	}
	if strings.TrimSpace(req.Text) == "" {
		writeErr(w, 400, "text is required")
		return
	}
	res := s.manager.HandleTurn(req.SessionID, req.Text)
	if res == nil {
		writeErr(w, 404, "session not found or expired")
		return
	}
	meta := res.Metadata
	if meta == nil {
		meta = map[string]any{}
	}
	writeJSON(w, 200, models.ProcessInputResponse{
		SessionID: req.SessionID, Message: res.Message, State: res.State,
		Intent: res.Intent, IsEmergency: res.IsEmergency, Metadata: meta,
	})
}

func (s *Server) endCall(w http.ResponseWriter, r *http.Request) {
	var req models.EndCallRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, 400, "invalid JSON")
		return
	}
	sess := s.manager.EndCall(req.SessionID)
	if sess == nil {
		writeErr(w, 404, "session not found")
		return
	}
	transcript := make([]map[string]any, 0, len(sess.Transcript))
	for _, t := range sess.Transcript {
		transcript = append(transcript, map[string]any{
			"role": t.Role, "text": t.Text, "ts": t.TS.Unix(),
		})
	}
	writeJSON(w, 200, models.EndCallResponse{
		SessionID: sess.ID, Message: "Call ended.", Transcript: transcript,
	})
}

// --- WebSocket -------------------------------------------------------
//
// Minimal hand-rolled WS implementation using the standard library is
// non-trivial; for v1 we proxy with a SSE-like long-poll fallback so we
// don't pull a websocket dep. The browser demo uses REST for its turns.
// (See note in README.md for production WS guidance — gorilla/websocket.)

func (s *Server) websocket(w http.ResponseWriter, r *http.Request) {
	writeErr(w, 501, "WebSocket not enabled in this build — use POST /process-input")
}

// --- Twilio Voice ----------------------------------------------------

func voiceFor(choice string) string {
	cfg := config.Get()
	pick := choice
	if pick == "" {
		pick = cfg.TwilioVoiceDefault
	}
	if strings.EqualFold(pick, "male") {
		return cfg.TwilioVoiceMale
	}
	return cfg.TwilioVoiceFemale
}

func twiml(body string) []byte { return []byte(body) }

func gatherXML(prompt, action, voice string) string {
	return `<?xml version="1.0" encoding="UTF-8"?>` +
		`<Response>` +
		`<Gather input="speech" action="` + escapeXML(action) + `" method="POST" speechTimeout="auto" language="en-US">` +
		`<Say voice="` + voice + `" language="en-US">` + escapeXML(prompt) + `</Say>` +
		`</Gather>` +
		`<Say voice="` + voice + `" language="en-US">I didn't hear anything. Please call back when you're ready. Goodbye.</Say>` +
		`<Hangup/></Response>`
}

func sayAndHangup(message, voice string) string {
	return `<?xml version="1.0" encoding="UTF-8"?>` +
		`<Response>` +
		`<Say voice="` + voice + `" language="en-US">` + escapeXML(message) + `</Say>` +
		`<Hangup/></Response>`
}

func escapeXML(s string) string {
	r := strings.NewReplacer(
		"&", "&amp;", "<", "&lt;", ">", "&gt;",
		`"`, "&quot;", "'", "&apos;",
	)
	return r.Replace(s)
}

func (s *Server) twilioVoice(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", 400)
		return
	}
	voicePref := r.URL.Query().Get("voice")
	from := r.FormValue("From")
	sess, greeting := s.manager.StartCall(from, "en-US", "")
	action := fmt.Sprintf("/twilio/gather?session_id=%s&voice=%s", sess.ID, voicePref)
	w.Header().Set("Content-Type", "application/xml")
	w.Write(twiml(gatherXML(greeting, action, voiceFor(voicePref))))
}

func (s *Server) twilioGather(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", 400)
		return
	}
	sid := r.URL.Query().Get("session_id")
	voicePref := r.URL.Query().Get("voice")
	v := voiceFor(voicePref)
	speech := strings.TrimSpace(r.FormValue("SpeechResult"))
	w.Header().Set("Content-Type", "application/xml")

	if speech == "" {
		action := fmt.Sprintf("/twilio/gather?session_id=%s&voice=%s", sid, voicePref)
		w.Write(twiml(gatherXML("I'm sorry, I didn't catch that. Could you say it again?", action, v)))
		return
	}

	res := s.manager.HandleTurn(sid, speech)
	if res == nil {
		w.Write(twiml(sayAndHangup("I'm sorry, your session has expired. Please call back.", v)))
		return
	}

	if res.IsEmergency || res.State == models.StateEnded {
		s.manager.EndCall(sid)
		w.Write(twiml(sayAndHangup(res.Message, v)))
		return
	}
	action := fmt.Sprintf("/twilio/gather?session_id=%s&voice=%s", sid, voicePref)
	w.Write(twiml(gatherXML(res.Message, action, v)))
}

func (s *Server) twilioStatus(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err == nil {
		log.Printf("twilio status callback: %v", r.PostForm)
	}
	w.WriteHeader(http.StatusNoContent)
}
