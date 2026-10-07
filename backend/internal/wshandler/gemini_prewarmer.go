package wshandler

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/globussoft/callified-backend/internal/dial"
	"github.com/globussoft/callified-backend/internal/realtime"
)

// preparedGeminiCall owns one Live connection while the Tata call is ringing.
// The same connection is attached to the answered call, so the model's opening
// turn and its later replies share one conversation and one voice.
type preparedGeminiCall struct {
	client  *realtime.Client
	sink    *preparedGeminiCallbacks
	audioIn chan []byte
	done    chan error
	cancel  context.CancelFunc
	timer   *time.Timer
}

func newPreparedGeminiCall(client *realtime.Client, sink *preparedGeminiCallbacks) (*preparedGeminiCall, context.Context) {
	ctx, cancel := context.WithCancel(context.Background())
	return &preparedGeminiCall{
		client: client, sink: sink, audioIn: make(chan []byte, 512),
		done: make(chan error, 1), cancel: cancel,
	}, ctx
}

// preparedGeminiCallbacks preserves model events generated before Tata opens
// the media stream, then replays them in order through the normal call handler.
type preparedGeminiCallbacks struct {
	mu                 sync.Mutex
	attached           *realtime.Callbacks
	events             []func(realtime.Callbacks)
	bufferedAudioBytes int
	firstAudio         chan struct{}
	once               sync.Once
}

const maxPreAnswerAudioBytes = 8 * 1024 * 1024

func newPreparedGeminiCallbacks() *preparedGeminiCallbacks {
	return &preparedGeminiCallbacks{firstAudio: make(chan struct{})}
}

func (s *preparedGeminiCallbacks) dispatch(event func(realtime.Callbacks)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.attached == nil {
		s.events = append(s.events, event)
		return
	}
	event(*s.attached)
}

func (s *preparedGeminiCallbacks) attach(cb realtime.Callbacks) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, event := range s.events {
		event(cb)
	}
	s.events = nil
	s.bufferedAudioBytes = 0
	s.attached = &cb
}

func (s *preparedGeminiCallbacks) callbacks(log *zap.Logger, leadID int64) realtime.Callbacks {
	return realtime.Callbacks{
		OnAudio: func(pcm []byte) {
			copyPCM := append([]byte(nil), pcm...)
			s.mu.Lock()
			if s.attached == nil {
				if s.bufferedAudioBytes+len(copyPCM) > maxPreAnswerAudioBytes {
					s.mu.Unlock()
					log.Warn("gemini live pre-answer audio buffer full", zap.Int64("lead_id", leadID))
					return
				}
				s.bufferedAudioBytes += len(copyPCM)
			}
			s.mu.Unlock()
			s.dispatch(func(cb realtime.Callbacks) {
				if cb.OnAudio != nil {
					cb.OnAudio(copyPCM)
				}
			})
			s.once.Do(func() { close(s.firstAudio) })
		},
		OnInterimInputTranscript: func(text string) {
			s.dispatch(func(cb realtime.Callbacks) {
				if cb.OnInterimInputTranscript != nil {
					cb.OnInterimInputTranscript(text)
				}
			})
		},
		OnInputTranscript: func(text string) {
			s.dispatch(func(cb realtime.Callbacks) {
				if cb.OnInputTranscript != nil {
					cb.OnInputTranscript(text)
				}
			})
		},
		OnOutputTranscript: func(text string) {
			s.dispatch(func(cb realtime.Callbacks) {
				if cb.OnOutputTranscript != nil {
					cb.OnOutputTranscript(text)
				}
			})
		},
		OnInterrupted: func() {
			s.dispatch(func(cb realtime.Callbacks) {
				if cb.OnInterrupted != nil {
					cb.OnInterrupted()
				}
			})
		},
		OnTurnComplete: func() {
			s.dispatch(func(cb realtime.Callbacks) {
				if cb.OnTurnComplete != nil {
					cb.OnTurnComplete()
				}
			})
		},
		OnCompleteCall: func(req realtime.CompleteCallRequest) bool {
			s.mu.Lock()
			defer s.mu.Unlock()
			if s.attached == nil || s.attached.OnCompleteCall == nil {
				return false
			}
			return s.attached.OnCompleteCall(req)
		},
		OnLanguageSwitch: func(language string) bool {
			s.mu.Lock()
			defer s.mu.Unlock()
			if s.attached == nil || s.attached.OnLanguageSwitch == nil {
				return false
			}
			return s.attached.OnLanguageSwitch(language)
		},
		OnKnowledgeQuery: func(query string) string {
			s.mu.Lock()
			defer s.mu.Unlock()
			if s.attached == nil || s.attached.OnKnowledgeQuery == nil {
				return ""
			}
			return s.attached.OnKnowledgeQuery(query)
		},
		OnStartupStage: func(stage string, elapsed time.Duration) {
			log.Info("gemini live pre-answer timing", zap.Int64("lead_id", leadID),
				zap.String("stage", stage), zap.Int64("since_gemini_run_ms", elapsed.Milliseconds()))
		},
	}
}

func (h *Handler) livePromptAndTimezone(sess *CallSession) (string, string) {
	timezone := "Asia/Kolkata"
	if h.db != nil && sess.OrgID > 0 {
		if configured, err := h.db.GetOrgTimezone(sess.OrgID); err == nil && strings.TrimSpace(configured) != "" {
			timezone = strings.TrimSpace(configured)
		}
	}
	location, err := time.LoadLocation(timezone)
	if err != nil {
		timezone = "Asia/Kolkata"
		location, _ = time.LoadLocation(timezone)
	}
	callNow := time.Now().In(location)
	prompt := sess.SystemPrompt + fmt.Sprintf(
		"\n\nCURRENT LOCAL DATE AND TIME: %s (%s). Follow the configured call flow for scheduling. "+
			"Collect both a customer-provided day/date and exact time, asking only for whichever detail is missing. "+
			"After both are available, complete and confirm the appointment without requesting an extra acknowledgement unless the call flow explicitly requires one. "+
			"When completing an appointment, pass appointment_date as YYYY-MM-DD and appointment_time as HH:MM in this timezone.",
		callNow.Format("2006-01-02 15:04"), timezone,
	)
	return prompt, timezone
}

// prepareGeminiCall begins the opening turn before the carrier answers. The
// bind closure keeps the prepared connection only after the dial has a SID.
func (h *Handler) prepareGeminiCall(data dial.CallData) func(string) {
	if h.promptBuilder == nil || data.IsBridge {
		return nil
	}
	sess := NewCallSession("gemini_pre_answer", nil, h.log)
	sess.LeadID, sess.CampaignID, sess.OrgID = data.LeadID, data.CampaignID, data.OrgID
	sess.LeadName, sess.LeadPhone, sess.Interest = data.LeadName, data.LeadPhone, data.Interest
	sess.TTSProvider, sess.TTSVoiceID = data.TTSProvider, data.TTSVoiceID
	sess.TTSLanguage = data.TTSLanguage
	sess.Language = data.Language
	if sess.Language == "" {
		sess.Language = data.TTSLanguage
	}
	initializeCtx, cancelInitialize := context.WithTimeout(context.Background(), 2*time.Minute)
	err := h.initializeCall(initializeCtx, sess)
	cancelInitialize()
	if err != nil || sess.GreetingText == "" ||
		!strings.EqualFold(sess.TTSProvider, "gemini_live") {
		return nil
	}
	livePrompt, _ := h.livePromptAndTimezone(sess)
	sink := newPreparedGeminiCallbacks()
	client := realtime.New(realtime.Config{
		URL: h.cfg.GeminiLiveURL, APIKey: firstNonEmpty(h.cfg.GeminiLiveAPIKey, h.cfg.GeminiAPIKey),
		AuthMode: h.cfg.GeminiLiveAuthMode, Model: h.cfg.GeminiLiveModel,
		Voice: firstNonEmpty(sess.TTSVoiceID, h.cfg.GeminiLiveVoice), SystemPrompt: livePrompt,
		Language: sess.Language, Greeting: sess.GreetingText,
	}, sink.callbacks(h.log, data.LeadID))
	// The connection survives the ringing-to-answer handoff. Only the separate
	// pre-answer timer and the attached call's lifecycle may cancel it.
	prepared, prepareCtx := newPreparedGeminiCall(client, sink)
	go func() { prepared.done <- client.Run(prepareCtx, prepared.audioIn) }()
	return func(callSID string) {
		if callSID == "" {
			prepared.cancel()
			return
		}
		prepared.timer = time.AfterFunc(90*time.Second, func() {
			if h.preparedLive.CompareAndDelete(callSID, prepared) {
				prepared.cancel()
			}
		})
		h.preparedLive.Store(callSID, prepared)
	}
}

func (h *Handler) takePreparedGeminiCall(callSID string) *preparedGeminiCall {
	value, ok := h.preparedLive.LoadAndDelete(callSID)
	if !ok {
		return nil
	}
	prepared := value.(*preparedGeminiCall)
	if prepared.timer != nil {
		prepared.timer.Stop()
	}
	select {
	case err := <-prepared.done:
		h.log.Warn("gemini live pre-answer session ended before attach", zap.String("call_sid", callSID), zap.Error(err))
		prepared.cancel()
		return nil
	default:
		return prepared
	}
}

func (p *preparedGeminiCall) runAttached(ctx context.Context, carrierReady <-chan struct{}, input <-chan []byte, attach func(), onTimeout func()) error {
	defer p.cancel()
	forwardCtx, cancelForward := context.WithCancel(ctx)
	defer cancelForward()
	// Tata's start envelope only confirms WebSocket setup. Wait for its first
	// actual media frame before replaying the prepared greeting, so the carrier
	// cannot drop it while its playback path is still coming up. This uses no
	// fixed delay. Customer PCM remains buffered until that greeting is ready.
	go func() {
		select {
		case <-carrierReady:
			attach()
		case <-forwardCtx.Done():
			return
		}
		select {
		case <-p.sink.firstAudio:
		case <-time.After(3 * time.Second):
			onTimeout()
		case <-forwardCtx.Done():
			return
		}
		for {
			select {
			case <-forwardCtx.Done():
				return
			case pcm, ok := <-input:
				if !ok {
					return
				}
				select {
				case p.audioIn <- pcm:
				case <-forwardCtx.Done():
					return
				}
			}
		}
	}()
	select {
	case err := <-p.done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}
