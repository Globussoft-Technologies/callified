package wshandler

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	"go.uber.org/zap"

	"github.com/globussoft/callified-backend/internal/audio"
	"github.com/globussoft/callified-backend/internal/config"
	"github.com/globussoft/callified-backend/internal/db"
	"github.com/globussoft/callified-backend/internal/llm"
	"github.com/globussoft/callified-backend/internal/metrics"
	"github.com/globussoft/callified-backend/internal/prompt"
	rstore "github.com/globussoft/callified-backend/internal/redis"
	"github.com/globussoft/callified-backend/internal/recording"
	"github.com/globussoft/callified-backend/internal/stt"
	"github.com/globussoft/callified-backend/internal/tts"
)

var upgrader = websocket.Upgrader{
	CheckOrigin:     func(r *http.Request) bool { return true },
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
}

// Handler serves the /media-stream and /ws/sandbox WebSocket endpoints.
type Handler struct {
	cfg           *config.Config
	promptBuilder *prompt.Builder    // Phase 3C: replaces gRPC InitializeCall
	recordingSvc  *recording.Service // Phase 4: replaces gRPC FinalizeCall
	store         *rstore.Store
	db            *db.DB        // for lead lookups when Redis pending-call info is sparse
	provider      *llm.Provider // Phase 0: native Go LLM
	ttsKeys       map[string]string
	log           *zap.Logger
	sessions      sync.Map // stream_sid → *CallSession (for monitor WebSocket)
	sessionsByCallSid sync.Map // call_sid → *CallSession (for monitor lookup during dial flow before stream_sid arrives)
}

// New creates a Handler wired to the provided dependencies.
func New(
	cfg *config.Config,
	promptBuilder *prompt.Builder,
	recordingSvc *recording.Service,
	store *rstore.Store,
	database *db.DB,
	log *zap.Logger,
) *Handler {
	var provider *llm.Provider
	if cfg.GeminiAPIKey != "" || cfg.GroqAPIKey != "" {
		provider = llm.NewProvider(cfg, log)
	}
	return &Handler{
		cfg:           cfg,
		promptBuilder: promptBuilder,
		recordingSvc:  recordingSvc,
		store:         store,
		db:            database,
		provider:      provider,
		ttsKeys: map[string]string{
			"elevenlabs": cfg.ElevenLabsAPIKey,
			"sarvam":     cfg.SarvamAPIKey,
			"smallest":   cfg.SmallestAPIKey,
		},
		log: log,
	}
}

// ServeHTTP handles both /media-stream (Exotel) and /ws/sandbox (browser sim).
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Validate query params BEFORE upgrading the WS so garbage from the
	// browser sandbox surfaces as a clean HTTP 400 (which the JS WebSocket API
	// turns into onerror/onclose-before-onopen) instead of opening a session
	// that then silently fails downstream — e.g. an unknown tts_provider would
	// previously upgrade the WS, fail tts.New() with a buried log warning, and
	// the sandbox would record audio with no TTS coming back.
	q := r.URL.Query()
	if msg := validateMediaStreamParams(q); msg != "" {
		http.Error(w, msg, http.StatusBadRequest)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		h.log.Warn("ws upgrade failed", zap.Error(err))
		return
	}
	defer conn.Close()

	// Extract initial identity from query params (may be overridden by "start" event)
	streamSid := q.Get("stream_sid")
	if streamSid == "" {
		streamSid = fmt.Sprintf("web_sim_%s_%d", q.Get("lead_id"), time.Now().UnixMilli())
	}

	sess := NewCallSession(streamSid, conn, h.log)
	// The browser-side web-sim sends `name` / `phone`; legacy callers may send
	// `lead_name` / `lead_phone`. Accept either so live-feed events render with
	// the lead label instead of the empty "()" we used to show.
	sess.LeadName = firstNonEmpty(q.Get("name"), q.Get("lead_name"))
	sess.LeadPhone = firstNonEmpty(q.Get("phone"), q.Get("lead_phone"))
	sess.Interest = q.Get("interest")
	if id := q.Get("lead_id"); id != "" {
		fmt.Sscanf(id, "%d", &sess.LeadID)
	}
	if id := q.Get("campaign_id"); id != "" {
		fmt.Sscanf(id, "%d", &sess.CampaignID)
	}
	if l := q.Get("tts_language"); l != "" {
		sess.Language = l
		sess.TTSLanguage = l
	}
	if p := q.Get("tts_provider"); p != "" {
		sess.TTSProvider = p
	}
	if v := q.Get("voice"); v != "" {
		sess.TTSVoiceID = v
	}

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	h.sessions.Store(sess.StreamSid, sess)
	defer func() {
		h.sessions.Delete(sess.StreamSid)
		if sess.CallSid != "" {
			h.sessionsByCallSid.Delete(sess.CallSid)
		}
	}()

	metrics.ActiveCalls.Inc()
	defer func() {
		metrics.ActiveCalls.Dec()
		metrics.CallDuration.Observe(time.Since(sess.CallStart).Seconds())
	}()

	// Web-sim path doesn't go through dial.Initiator, so the live-feed never
	// gets a DIALING entry — operators only saw CONNECTED + COMPLETED with
	// empty "()". Emit one here so the activity panel renders the same
	// 3-event sequence (DIALING → CONNECTED → COMPLETED) as a real dial.
	if sess.CampaignID > 0 && strings.HasPrefix(streamSid, "web_sim_") {
		h.store.EmitCampaignEvent(ctx, sess.CampaignID, sess.LeadName, sess.LeadPhone,
			"dialing", "via web-sim")
	}

	// --- Initialize call via gRPC (get system prompt + voice config) ---
	if err := h.initializeCall(ctx, sess); err != nil {
		h.log.Error("InitializeCall failed", zap.Error(err))
		// Continue with defaults — don't abort the call
	}

	// --- Voice consistency cache (lead_voice:{id}, 90-day TTL) ---
	// Same lead reliably hears the same agent voice across calls (ported from
	// main-branch ws_handler.py 4aa3fa3). Best-effort: errors are swallowed.
	if h.store != nil && sess.LeadID != 0 && sess.TTSVoiceID != "" {
		voice, fromCache := h.store.ResolveLeadVoice(ctx, sess.LeadID, sess.TTSProvider, sess.TTSVoiceID)
		if fromCache && voice != sess.TTSVoiceID {
			h.log.Info("voice cache: using cached voice",
				zap.Int64("lead_id", sess.LeadID),
				zap.String("from", sess.TTSVoiceID),
				zap.String("to", voice))
			sess.TTSVoiceID = voice
		}
	}

	// --- Select TTS provider ---
	// Store the instance on the session so runTTSWorker (which reads it every
	// sentence) and the greeting dispatch can both use it. Previously this was
	// a closure variable, but the worker now lives outside this function.
	ttsProvider, err := tts.New(sess.TTSProvider, h.ttsKeys)
	if err != nil {
		h.log.Warn("TTS provider unavailable", zap.Error(err), zap.String("provider", sess.TTSProvider))
	}
	if ttsProvider != nil {
		sess.SetTTSInstance(ttsProvider)
	}

	// --- Start Deepgram STT client ---
	// Build the transcript callback once and share it between single and dual
	// clients. Dual mode runs primary (multi/lang) + secondary (hi) in parallel
	// and merges by confidence within 300ms — recovers Hindi misclassified by
	// Deepgram's "multi" mode. Mirrors main-branch ws_handler.py 4aa3fa3.
	onTranscript := func(text string) {
		if first, elapsed := sess.MarkSTTFirst(); first {
			metrics.STTFirstByteLatency.Observe(elapsed)
		}
		if sess.HangupRequested() {
			return
		}
		// Block transcript only while TTS is actively playing and barge-in hasn't
		// fired yet. Post-TTS gate is 200ms (down from 1000ms) — keeps echo out
		// while letting quick replies like "yes" / "no" through.
		if !sess.IsBargeInActive() && (sess.IsTTSPlaying() || sess.MsSinceTTSEnd() < 200) {
			sess.Log.Debug("transcript dropped: TTS cooldown",
				zap.Bool("tts_playing", sess.IsTTSPlaying()),
				zap.Int64("ms_since_tts_end", sess.MsSinceTTSEnd()))
			return
		}
		// Guard against send on closed channel if session tore down mid-STT.
		select {
		case sess.Transcripts <- text:
		case <-ctx.Done():
		}
	}
	onSpeechStarted := func() {
		sess.Log.Info("barge-in: SpeechStarted",
			zap.Bool("tts_playing", sess.IsTTSPlaying()),
			zap.Bool("is_web_sim", sess.IsWebSim),
			zap.Bool("is_exotel", sess.IsExotel),
		)
		if sess.IsTTSPlaying() {
			metrics.BargeIns.Inc()
		}
		// Always set flag + drain — even when TTS is between sentences the
		// TTSSentences buffer may still hold queued text that must be dropped.
		sess.SetBargeIn(true)
		n := sess.DrainTTSSentences()
		sess.Log.Info("barge-in: draining", zap.Int("drained", n), zap.Bool("tts_was_playing", sess.IsTTSPlaying()))
		// Safety: clear flag after 3s in case LLM never responds.
		go func() {
			time.Sleep(3 * time.Second)
			sess.SetBargeIn(false)
		}()
		sess.CancelActiveTTS()
		if sess.IsExotel {
			sendClearEvent(sess)
		} else if sess.IsWebSim {
			frame, _ := json.Marshal(map[string]string{"type": "clear"})
			_ = sess.SendText(frame)
		}
	}

	var wg sync.WaitGroup

	// STT and greeting must be started *after* sess.Language is final. For
	// web-sim that's already true (URL params populated everything). For real
	// Exotel calls the WS connects with empty params and the campaign's
	// language only arrives via Redis on the "start" event — starting STT or
	// sending the greeting before then locks them to the wrong language for
	// the duration of the call (Deepgram doesn't accept mid-stream language
	// switches, and the greeting is one-shot).
	//
	// Solution: make both deferrable via closures that handleStartEvent can
	// trigger after Redis hydration completes, and only fire them now when
	// the URL params already gave us enough.
	var sttStarted atomic.Bool
	startSTT := func() {
		// Idempotent: web-sim invokes this directly at startup; handleStartEvent
		// invokes it again after Redis hydration if it wasn't fired yet.
		// Without the atomic gate a stray Exotel "start" event mid-call would
		// spawn a second Deepgram connection on the same audio channel.
		if sttStarted.Swap(true) {
			return
		}
		// g2: STT goroutine — DualClient for non-Hindi/non-English Indian languages,
		// single client otherwise. Hindi already uses Deepgram's dedicated nova-2
		// hi model; English uses nova-2 en — no benefit from a parallel hi connection.
		useDualSTT := sess.Language != "hi" && sess.Language != "en" && sess.Language != ""
		wg.Add(1)
		if useDualSTT {
			dual := stt.NewDualClient(h.cfg.DeepgramAPIKey, sess.Language, "hi", h.log)
			dual.OnTranscript = onTranscript
			dual.OnSpeechStarted = onSpeechStarted
			go func() {
				defer wg.Done()
				dual.Run(ctx, sess.AudioIn)
			}()
		} else {
			dgClient := stt.NewClient(h.cfg.DeepgramAPIKey, sess.Language, h.log)
			dgClient.OnTranscript = onTranscript
			dgClient.OnSpeechStarted = onSpeechStarted
			go func() {
				defer wg.Done()
				dgClient.Run(ctx, sess.AudioIn)
			}()
		}
	}
	sess.StartSTT = startSTT

	// g4: Pipeline orchestrator
	wg.Add(1)
	go func() {
		defer wg.Done()
		runPipeline(ctx, sess, h.provider, h.store)
	}()

	// g5: TTS worker. Reads the provider from sess.TTSInstance() on each
	// sentence; the worker no-ops with a warning if no provider is loaded.
	// Launched unconditionally so that if the provider becomes available
	// mid-call (e.g. after Redis hydration of a campaign with a different
	// provider), synthesis resumes without needing to relaunch the worker.
	wg.Add(1)
	go func() {
		defer wg.Done()
		runTTSWorker(ctx, sess)
	}()

	// Greeting closure — dispatched here for web-sim (we already have the
	// language from URL params), or from handleStartEvent for Exotel after
	// Redis hydration finalises the language. Reads sess.TTSInstance() so
	// it picks up whatever provider was actually configured.
	sendGreeting := func() {
		if !sess.TrySetGreeting() || sess.GreetingText == "" {
			return
		}
		prov := sess.TTSInstance()
		if prov == nil {
			return
		}
		go synthesizeAndSend(ctx, sess, prov, sess.GreetingText)
		// Also broadcast the greeting to monitors / Sandbox panel and store it
		// in chat history so the AI's opening line shows up alongside the
		// user's reply (issue #33). Without this, the Live Transcripts panel
		// only ever showed turns starting from the user's first utterance.
		sess.BroadcastTranscript("agent", sess.GreetingText)
		sess.AppendHistory("model", sess.GreetingText)
	}
	sess.SendGreeting = sendGreeting

	// For web-sim the URL params have already given us the language, voice,
	// and lead context — start STT and send the greeting now. For Exotel the
	// URL is empty until the "start" event lands; handleStartEvent will fire
	// these once Redis hydration has populated sess.Language / TTSInstance().
	hasLanguage := sess.Language != ""
	if hasLanguage {
		startSTT()
		sendGreeting()
	}

	// --- g1: WebSocket message loop ---
	done := h.messageLoop(ctx, sess)
	cancel() // signal all goroutines to stop

	// Close AudioIn so the STT send goroutine exits its range loop.
	// Do NOT close sess.Transcripts — the Deepgram receive goroutine may still
	// deliver a final transcript after cancel(), and sending to a closed channel
	// panics. runPipeline exits via ctx.Done() instead.
	close(sess.AudioIn)

	wg.Wait()

	if !done {
		// Abnormal close (network error) — still finalize
	}

	h.finalizeCall(context.Background(), sess)
}

// messageLoop reads WebSocket frames until the connection closes or a "stop"
// event is received. Returns true on clean stop, false on error.
func (h *Handler) messageLoop(ctx context.Context, sess *CallSession) bool {
	for {
		msgType, msg, err := sess.WS.ReadMessage()
		if err != nil {
			return false
		}
		switch msgType {
		case websocket.BinaryMessage:
			h.handleBinaryFrame(sess, msg)
		case websocket.TextMessage:
			if stop := h.handleTextFrame(ctx, sess, msg); stop {
				return true
			}
		}
	}
}

func (h *Handler) handleBinaryFrame(sess *CallSession, data []byte) {
	if sess.HangupRequested() {
		return
	}
	var pcm []byte
	if sess.IsExotel {
		// Echo cancellation: check ulaw frame before decoding
		if sess.EchoCanceller.IsEcho(data) {
			metrics.EchoSuppressions.Inc()
			return
		}
		pcm = audio.UlawToPCM(data)
	} else {
		pcm = data // web sim sends PCM directly
	}
	sess.AppendMicChunk(pcm)
	// Energy VAD: trigger barge-in immediately when user speaks during TTS.
	// Does not depend on Deepgram SpeechStarted (which requires a paid plan tier).
	// Fire energy VAD while TTS is playing OR within 500ms of it ending.
	// The 500ms window catches users who speak the instant the agent finishes
	// (their audio may still be in-flight when IsTTSPlaying flips to false).
	recentTTS := sess.IsTTSPlaying() || sess.MsSinceTTSEnd() < 500
	if recentTTS && !sess.IsBargeInActive() && pcmEnergy(pcm) > bargeInEnergyThreshold {
		if sess.TriggerBargeIn() {
			sess.Log.Info("barge-in: energy VAD triggered", zap.Int64("energy", pcmEnergy(pcm)))
		}
	}
	select {
	case sess.AudioIn <- pcm:
	default: // drop if buffer full
	}
}

func (h *Handler) handleTextFrame(ctx context.Context, sess *CallSession, data []byte) (stop bool) {
	var event map[string]interface{}
	if err := json.Unmarshal(data, &event); err != nil {
		return false
	}
	switch event["event"] {
	case "connected":
		// Exotel handshake ack — ignore
	case "start":
		h.handleStartEvent(ctx, sess, event)
	case "media":
		h.handleMediaEvent(sess, event)
	case "stop":
		return true
	}
	return false
}

func (h *Handler) handleStartEvent(ctx context.Context, sess *CallSession, event map[string]interface{}) {
	// Extract stream_sid and call_sid from the "start" event. Exotel sometimes
	// sends snake_case (call_sid / stream_sid) and sometimes Twilio-style
	// camelCase (callSid / streamSid) depending on the integration; read both
	// so the Redis-pending-call lookup that hydrates lead name/phone never
	// silently misses on field-name casing.
	if startData, ok := event["start"].(map[string]interface{}); ok {
		if sid := pickStr(startData, "streamSid", "stream_sid", "StreamSid"); sid != "" {
			sess.StreamSid = sid
			sess.UpdateStreamType()
		}
		if callSid := pickStr(startData, "callSid", "call_sid", "CallSid"); callSid != "" {
			sess.CallSid = callSid
			h.sessionsByCallSid.Store(callSid, sess)
			// Redis lookup precedence:
			//   1) under the carrier-issued call_sid (set by dial.Initiator)
			//   2) under "phone:<E164>" (set by manual-call web-sim mode)
			//   3) under "latest" (last-resort fallback)
			info, ok := h.store.GetPendingCall(ctx, callSid)
			if !ok {
				if phone := pickStr(startData, "from", "From", "to", "To"); phone != "" {
					info, ok = h.store.GetPendingCall(ctx, "phone:"+phone)
				}
			}
			if !ok {
				info, ok = h.store.GetPendingCall(ctx, "latest")
			}
			if ok {
				// Only overwrite when Redis has something — otherwise we wipe
				// good values (e.g. set from query params on web-sim) with
				// empty strings from a stale "latest" key.
				if info.Name != "" {
					sess.LeadName = info.Name
				}
				if info.Phone != "" {
					sess.LeadPhone = info.Phone
				}
				if info.LeadID != 0 {
					sess.LeadID = info.LeadID
				}
				if info.Interest != "" {
					sess.Interest = info.Interest
				}
				if info.CampaignID != 0 {
					sess.CampaignID = info.CampaignID
				}
				if info.OrgID != 0 {
					sess.OrgID = info.OrgID
				}
				if info.TTSProvider != "" {
					sess.TTSProvider = info.TTSProvider
				}
				if info.TTSVoiceID != "" {
					sess.TTSVoiceID = info.TTSVoiceID
				}
				if info.TTSLanguage != "" {
					sess.TTSLanguage = info.TTSLanguage
					sess.Language = info.TTSLanguage
				}
				// Rebuild SystemPrompt and GreetingText now that we know the
				// real campaign/org/lead. The initial initializeCall ran
				// before the start event with all-zero IDs (Exotel's Passthru
				// applet doesn't forward our query params), so it produced a
				// generic prompt with no language directive — Sarvam's Indian
				// voices then default to Hindi, and the LLM follows suit even
				// when the campaign is set to English.
				if h.promptBuilder != nil {
					_ = h.initializeCall(ctx, sess)
				}
				// Re-create the TTS provider in case the original startup picked
				// the wrong one (Exotel calls hit tts.New("") which falls back
				// to ElevenLabs — wrong if the campaign uses sarvam/smallest).
				// The TTS worker reads sess.TTSInstance() on every sentence, so
				// swapping it here makes subsequent synthesis use the correct
				// provider without restarting the goroutine.
				if sess.TTSProvider != "" {
					if newProv, err := tts.New(sess.TTSProvider, h.ttsKeys); err == nil && newProv != nil {
						sess.SetTTSInstance(newProv)
					}
				}
				// Fire the deferred STT + greeting now that the language is
				// final. ServeHTTP wired these closures and skipped the
				// immediate startup path because URL params didn't carry a
				// language. StartSTT is a no-op the second time (web-sim
				// already invoked it directly); SendGreeting is gated by
				// TrySetGreeting so it's also single-shot.
				if sess.StartSTT != nil && sess.Language != "" {
					sess.StartSTT()
					sess.StartSTT = nil // prevent double-start on a second start event
				}
				if sess.SendGreeting != nil {
					sess.SendGreeting()
				}
			}
		}
	}
	// Also accept top-level stream_sid (snake_case or camel)
	if sid := pickStr(event, "stream_sid", "streamSid"); sid != "" && sess.StreamSid == "" {
		sess.StreamSid = sid
		sess.UpdateStreamType()
	}

	// Live-feed: tell the campaign detail page that audio is flowing.
	// Fires on first "start" event so the Live Campaign Activity panel
	// shows one entry per connected call (web-sim + real Exotel both
	// send `start`, so both paths contribute to the live feed).
	if sess.CampaignID > 0 {
		name, phone := h.leadLabel(ctx, sess)
		h.store.EmitCampaignEvent(ctx, sess.CampaignID, name, phone,
			"connected", "audio stream opened")
	}
}

// bargeInEnergyThreshold is the mean-square PCM energy level above which we
// treat incoming mic audio as speech and trigger barge-in. int16 PCM has a max
// value of 32767; typical speech RMS is 1000–8000 (mean-square 1e6–64e6).
// 150_000 ≈ RMS 387 — catches soft voices while staying above mic noise floor.
const bargeInEnergyThreshold int64 = 150_000

// pcmEnergy returns the mean-square energy of a PCM16LE byte slice.
func pcmEnergy(pcm []byte) int64 {
	n := len(pcm) / 2
	if n == 0 {
		return 0
	}
	var sum int64
	for i := 0; i+1 < len(pcm); i += 2 {
		s := int64(int16(uint16(pcm[i]) | uint16(pcm[i+1])<<8))
		sum += s * s
	}
	return sum / int64(n)
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// pickStr returns the first non-empty string value found at any of the given
// keys in m. Used to tolerate camelCase / snake_case / PascalCase variants
// that Exotel and Twilio send for the same field.
func pickStr(m map[string]interface{}, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k].(string); ok && v != "" {
			return v
		}
	}
	return ""
}

// leadLabel returns the (name, phone) pair to display in live-feed events.
// Falls back to a DB lookup when the session struct has empty values — this
// happens when the Redis pending-call entry hasn't been written or doesn't
// carry name/phone (e.g. some Exotel start events arrive before the dialer
// publishes lead context). Without this fallback, CONNECTED and COMPLETED
// events render as "() — COMPLETED" in the activity panel.
func (h *Handler) leadLabel(ctx context.Context, sess *CallSession) (string, string) {
	name, phone := sess.LeadName, sess.LeadPhone
	if name != "" && phone != "" {
		return name, phone
	}
	if h.db == nil {
		return name, phone
	}
	// Try by lead_id first (cheapest — primary key).
	if sess.LeadID != 0 {
		if lead, err := h.db.GetLeadByID(sess.LeadID); err == nil && lead != nil {
			if name == "" {
				name = strings.TrimSpace(lead.FirstName + " " + lead.LastName)
				sess.LeadName = name
			}
			if phone == "" {
				phone = lead.Phone
				sess.LeadPhone = phone
			}
			if name != "" && phone != "" {
				return name, phone
			}
		}
	}
	// Last resort: lookup by phone. Covers the Exotel case where the carrier's
	// call_sid didn't match the Redis key (stale TTL, race, or field-name
	// mismatch) and we lost the lead_id, but the session still knows the
	// phone number from the start event.
	if phone != "" {
		if lead, err := h.db.GetLeadByPhone(phone); err == nil && lead != nil && name == "" {
			name = strings.TrimSpace(lead.FirstName + " " + lead.LastName)
			sess.LeadName = name
			if sess.LeadID == 0 {
				sess.LeadID = lead.ID
			}
		}
	}
	return name, phone
}

func (h *Handler) handleMediaEvent(sess *CallSession, event map[string]interface{}) {
	if sess.HangupRequested() {
		return
	}
	mediaData, _ := event["media"].(map[string]interface{})
	if mediaData == nil {
		return
	}
	payload, _ := mediaData["payload"].(string)
	if payload == "" {
		return
	}
	raw, err := base64.StdEncoding.DecodeString(payload)
	if err != nil || len(raw) == 0 {
		return
	}

	var pcm []byte
	if sess.IsExotel {
		if sess.EchoCanceller.IsEcho(raw) {
			metrics.EchoSuppressions.Inc()
			return
		}
		pcm = audio.UlawToPCM(raw)
	} else {
		pcm = raw
	}
	sess.AppendMicChunk(pcm)
	// Energy VAD: trigger barge-in immediately when user speaks during TTS.
	// Fire energy VAD while TTS is playing OR within 500ms of it ending.
	// The 500ms window catches users who speak the instant the agent finishes
	// (their audio may still be in-flight when IsTTSPlaying flips to false).
	recentTTS := sess.IsTTSPlaying() || sess.MsSinceTTSEnd() < 500
	if recentTTS && !sess.IsBargeInActive() && pcmEnergy(pcm) > bargeInEnergyThreshold {
		if sess.TriggerBargeIn() {
			sess.Log.Info("barge-in: energy VAD triggered", zap.Int64("energy", pcmEnergy(pcm)))
		}
	}
	select {
	case sess.AudioIn <- pcm:
	default:
	}

	// Relay a copy of the caller's inbound audio to any attached monitors.
	if sess.hasMonitors() {
		format := "pcm16_8k"
		if sess.IsExotel {
			format = "ulaw_8k"
		}
		sess.BroadcastAudio("user", payload, format)
	}
}

// initializeCall populates the session's system prompt and voice config.
// Phase 4: uses the native Go prompt builder exclusively (gRPC removed).
func (h *Handler) initializeCall(ctx context.Context, sess *CallSession) error {
	if h.promptBuilder == nil {
		return nil // no-op when DB is unavailable (dev/test)
	}
	callCtx, err := h.promptBuilder.BuildCallContext(ctx, sess.OrgID, sess.CampaignID, sess.LeadID, sess.Language)
	if err != nil {
		h.log.Warn("promptBuilder.BuildCallContext failed, proceeding with defaults", zap.Error(err))
		return nil
	}
	sess.SystemPrompt = callCtx.SystemPrompt
	sess.GreetingText = callCtx.GreetingText
	// Only fill in TTS fields the caller didn't already set via query params.
	// The Sandbox / web-sim flow passes ?tts_provider=&voice=&tts_language=
	// to override the org default for one session — without this guard, the
	// org default clobbers the explicit selection and the user always hears
	// the same default voice regardless of what they pick. (issue: Sandbox
	// "voice picker doesn't change the voice")
	if sess.TTSProvider == "" && callCtx.TTSProvider != "" {
		sess.TTSProvider = callCtx.TTSProvider
	}
	if sess.TTSVoiceID == "" && callCtx.TTSVoiceID != "" {
		sess.TTSVoiceID = callCtx.TTSVoiceID
	}
	if sess.TTSLanguage == "" && callCtx.TTSLanguage != "" {
		sess.TTSLanguage = callCtx.TTSLanguage
		sess.Language = callCtx.TTSLanguage // drives Deepgram language + LLM prompt language
	}
	if callCtx.AgentName != "" {
		sess.AgentName = callCtx.AgentName
	}
	// Swap the persona name in the greeting when the session's voice differs
	// from whatever the prompt builder used. Two cases:
	//   1. Org has a default voice (e.g. "aditya") and the Sandbox picked a
	//      different one (e.g. "mithali"): swap "Aditya" → "Mithali".
	//   2. Org has NO default voice configured: the prompt builder rendered
	//      the greeting with the empty-voice fallback ("Arjun"). The Sandbox
	//      almost always hits this path, so without the swap every voice
	//      ends up greeted as "Arjun".
	if sess.TTSVoiceID != "" && sess.TTSVoiceID != callCtx.TTSVoiceID {
		oldName := prompt.AgentPersonaName(callCtx.TTSVoiceID, sess.Language)
		newName := prompt.AgentPersonaName(sess.TTSVoiceID, sess.Language)
		if oldName != "" && newName != "" && oldName != newName {
			sess.GreetingText = strings.ReplaceAll(sess.GreetingText, oldName, newName)
		}
	}
	return nil
}

// finalizeCall runs post-call processing (Phase 4: native Go, no gRPC).
func (h *Handler) finalizeCall(ctx context.Context, sess *CallSession) {
	micChunks, ttsChunks := sess.DrainRecordingBuffers()
	wavBytes := audio.BuildStereoWAV(micChunks, ttsChunks)

	// Live-feed: emit completion so the Live Campaign Activity panel closes
	// out the entry. For web-sim calls this is the ONLY event that fires
	// (web-sim never goes through the Exotel webhook that emits dialing /
	// no-answer / etc.), so without it the panel stays empty during testing.
	if sess.CampaignID > 0 {
		durS := int(time.Since(sess.CallStart).Seconds())
		name, phone := h.leadLabel(ctx, sess)
		h.store.EmitCampaignEvent(ctx, sess.CampaignID, name, phone,
			"completed", fmt.Sprintf("%ds call", durS))
	}

	h.store.CleanupCall(ctx, sess.StreamSid)
	h.store.DeletePendingCall(ctx, sess.CallSid)

	if h.recordingSvc == nil {
		return // no-op when DB is unavailable
	}

	req := recording.SaveRequest{
		StreamSid:   sess.StreamSid,
		CallSid:     sess.CallSid,
		LeadID:      sess.LeadID,
		CampaignID:  sess.CampaignID,
		OrgID:       sess.OrgID,
		LeadPhone:   sess.LeadPhone,
		AgentName:   sess.AgentName,
		TTSLanguage: sess.TTSLanguage,
		ChatHistory: sess.HistorySnapshot(),
		DurationS:   float32(time.Since(sess.CallStart).Seconds()),
		StereoWav:   wavBytes,
	}
	go h.recordingSvc.SaveAndAnalyze(ctx, req)
}
