// Package metrics provides Prometheus instrumentation for the Go audio service.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// ActiveCalls is the number of currently active WebSocket call sessions.
	ActiveCalls = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "callified_active_calls",
		Help: "Number of currently active WebSocket call sessions.",
	})

	// CallDuration records total call duration in seconds.
	CallDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "callified_call_duration_seconds",
		Help:    "Total call duration from WebSocket connect to close.",
		Buckets: prometheus.DefBuckets,
	})

	// STTFirstByteLatency records time from call connect to first STT transcript.
	STTFirstByteLatency = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "callified_stt_ttfb_seconds",
		Help:    "Latency from call connect to first Deepgram transcript.",
		Buckets: prometheus.DefBuckets,
	})

	// LLMFirstByteLatency records time from user transcript to first LLM sentence chunk.
	LLMFirstByteLatency = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "callified_llm_ttfb_seconds",
		Help:    "Latency from user transcript to first streamed LLM sentence chunk.",
		Buckets: prometheus.DefBuckets,
	})

	// TTSFirstByteLatency records time from sentence to first TTS audio chunk.
	TTSFirstByteLatency = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "callified_tts_ttfb_seconds",
		Help:    "Latency from TTS sentence submission to first PCM audio chunk.",
		Buckets: prometheus.DefBuckets,
	})

	// GRPCLatency records the full round-trip duration of gRPC ProcessTranscript calls.
	GRPCLatency = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "callified_grpc_latency_seconds",
		Help:    "Full round-trip latency of Go→Python gRPC ProcessTranscript calls.",
		Buckets: prometheus.DefBuckets,
	})

	// HangupWait records the actual playback drain wait before WebSocket close.
	HangupWait = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "callified_hangup_wait_seconds",
		Help:    "Seconds waited for audio playback drain before HANGUP close.",
		Buckets: prometheus.DefBuckets,
	})

	// EchoSuppressions counts audio frames suppressed by the echo canceller.
	EchoSuppressions = promauto.NewCounter(prometheus.CounterOpts{
		Name: "callified_echo_suppressions_total",
		Help: "Total number of audio frames suppressed as echo by the echo canceller.",
	})

	// BargeIns counts user interruptions of active TTS playback.
	BargeIns = promauto.NewCounter(prometheus.CounterOpts{
		Name: "callified_barge_in_total",
		Help: "Total number of user barge-in events (speech detected during TTS).",
	})

	// DialAttemptsTotal counts outbound dial attempts by provider and outcome.
	// outcome label values: success, dnd, call_hours, insufficient_credits,
	// invalid_credentials, provider_error, unknown.
	DialAttemptsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "callified_dial_attempts_total",
		Help: "Total outbound dial attempts by provider and outcome.",
	}, []string{"provider", "outcome"})

	// CallStateTransitionsTotal counts valid call-state machine transitions.
	CallStateTransitionsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "callified_call_state_transitions_total",
		Help: "Total call state transitions by from-state and to-state.",
	}, []string{"from", "to"})

	// LLMTokenUsageTotal counts input and output tokens reported by the LLM provider.
	// direction label values: input, output.
	LLMTokenUsageTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "callified_llm_token_usage_total",
		Help: "Total LLM token usage by provider and direction.",
	}, []string{"provider", "direction"})

	// LLMResponseLatency records full LLM response latency for non-streaming calls.
	LLMResponseLatency = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "callified_llm_response_seconds",
		Help:    "Full LLM response latency from request to final token.",
		Buckets: prometheus.DefBuckets,
	}, []string{"provider"})

	// QueueDepth reports the current length of Redis-backed queues.
	QueueDepth = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "callified_queue_depth",
		Help: "Current number of items in a Redis-backed queue.",
	}, []string{"queue"})

	// WebSocketConnections tracks currently open WebSocket connections by endpoint type.
	// type label values: media, sandbox, monitor, agent.
	WebSocketConnections = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "callified_websocket_connections",
		Help: "Number of currently open WebSocket connections by endpoint type.",
	}, []string{"type"})

	// WebSocketConnectionDuration records WebSocket session duration by endpoint type.
	WebSocketConnectionDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "callified_websocket_duration_seconds",
		Help:    "WebSocket session duration from upgrade to close by endpoint type.",
		Buckets: prometheus.DefBuckets,
	}, []string{"type"})
)
