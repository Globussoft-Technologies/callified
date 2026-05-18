package api

import (
	"net/http"

	receptionistsrv "github.com/globussoft/callified-backend/internal/receptionist/server"
)

// newReceptionistHandler returns an http.Handler that serves the embedded
// AI Receptionist (formerly the standalone go-receptionist binary).
// When called as a method on *Server (with a live db), we inject a
// PastConversationSink so receptionist calls land in call_transcripts
// and appear in the dashboard's Past Conversations modal alongside
// campaign calls.
//
// We use http.StripPrefix because the receptionist's internal mux registers
// its routes at root paths (e.g. POST /tts), not /api/receptionist/tts. The
// HTML demo at "/" uses relative fetch paths so it works correctly under
// any prefix.
func (s *Server) newReceptionistHandler() http.Handler {
	opts := []receptionistsrv.Option{}
	if s != nil && s.db != nil {
		opts = append(opts, receptionistsrv.WithPastConversationSink(&receptionistSink{s: s}))
	}
	return http.StripPrefix("/api/receptionist", receptionistsrv.New(opts...).Handler())
}

// NewReceptionistHandler is the exported variant for callers outside the
// api package — used by cmd/audiod when MySQL is unavailable so local
// dev can still serve /api/receptionist/* without spinning up a DB.
// Always built without a sink because no DB is available in this path.
func NewReceptionistHandler() http.Handler {
	return http.StripPrefix("/api/receptionist", receptionistsrv.New().Handler())
}
