package api

import (
	"net/http"

	receptionistsrv "github.com/globussoft/callified-backend/internal/receptionist/server"
)

// receptionistHandler returns an http.Handler that serves the embedded
// AI Receptionist (formerly the standalone go-receptionist binary). The
// receptionist is now compiled directly into audiod — no separate process,
// no /tmp/ai-receptionist binary, no per-environment .env. All routes
// (/, /tts, /start-call, /process-input, /end-call, /doctors, /dispatch,
// /twilio/*) are mounted under /api/receptionist/* so testgo's existing
// nginx /api/ → :8011 rule routes them straight to this handler with no
// nginx config change required.
//
// We use http.StripPrefix because the receptionist's internal mux registers
// its routes at root paths (e.g. POST /tts), not /api/receptionist/tts. The
// HTML demo at "/" uses relative fetch paths so it works correctly under
// any prefix.
func newReceptionistHandler() http.Handler {
	return http.StripPrefix("/api/receptionist", receptionistsrv.New().Handler())
}
