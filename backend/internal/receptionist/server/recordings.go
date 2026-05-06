package server

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/globussoft/callified-backend/internal/receptionist/recordings"
)

// uploadMaxBytes caps a single recording upload. 30 MB at Opus ~24kbps
// is roughly 2.7 hours — generous for a demo call but bounds disk abuse.
const uploadMaxBytes = 30 * 1024 * 1024

// recordingID returns a short random hex id (16 chars) for a recording.
// Not cryptographically critical — collisions inside a recorder's
// directory are vanishingly unlikely at any realistic call volume.
func recordingID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// recordingsList: GET /recordings?recorder_id=...
//
// Returns this recorder's recordings (id, ts, duration, transcript)
// without the audio. Audio is fetched separately via /recordings/{id}/audio.
func (s *Server) recordingsList(w http.ResponseWriter, r *http.Request) {
	if s.recordings == nil {
		writeErr(w, 503, "recordings store not initialized")
		return
	}
	rid := r.URL.Query().Get("recorder_id")
	if err := recordings.SafeID(rid); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	items, err := s.recordings.List(rid)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"recordings": items})
}

// recordingsUpload: POST /recordings (multipart/form-data)
//
// Form fields:
//   - recorder_id (required) — opaque per-browser id
//   - session_id  (optional) — receptionist session id this came from
//   - duration_ms (optional) — wallclock length in ms
//   - transcript  (optional) — JSON-encoded []TranscriptLine
//   - audio       (required) — the combined webm/opus blob
//
// Stores the audio + a JSON sidecar and returns the new recording id.
func (s *Server) recordingsUpload(w http.ResponseWriter, r *http.Request) {
	if s.recordings == nil {
		writeErr(w, 503, "recordings store not initialized")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, uploadMaxBytes)
	if err := r.ParseMultipartForm(uploadMaxBytes); err != nil {
		writeErr(w, 400, "multipart parse: "+err.Error())
		return
	}
	rid := r.FormValue("recorder_id")
	if err := recordings.SafeID(rid); err != nil {
		writeErr(w, 400, err.Error())
		return
	}

	file, header, err := r.FormFile("audio")
	if err != nil {
		writeErr(w, 400, "audio file required: "+err.Error())
		return
	}
	defer file.Close()

	mime := header.Header.Get("Content-Type")
	if mime == "" {
		mime = "audio/webm"
	}

	durationMS := 0
	if v := r.FormValue("duration_ms"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			durationMS = n
		}
	}

	var transcript []recordings.TranscriptLine
	if t := r.FormValue("transcript"); t != "" {
		if err := json.Unmarshal([]byte(t), &transcript); err != nil {
			writeErr(w, 400, "transcript must be JSON array: "+err.Error())
			return
		}
	}

	id := recordingID()
	meta := recordings.Meta{
		ID:         id,
		RecorderID: rid,
		SessionID:  r.FormValue("session_id"),
		CreatedAt:  time.Now().UTC(),
		DurationMS: durationMS,
		AudioMIME:  mime,
		Transcript: transcript,
	}
	if err := s.recordings.Save(meta, file); err != nil {
		writeErr(w, 500, "save: "+err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"id": id, "created_at": meta.CreatedAt})
}

// recordingsAudio: GET /recordings/{id}/audio?recorder_id=...
//
// Streams the webm bytes inline so an <audio> element can play them
// directly. Range requests aren't currently supported (browsers cope
// fine for short demo clips); add http.ServeFile if seeks become an issue.
func (s *Server) recordingsAudio(w http.ResponseWriter, r *http.Request) {
	if s.recordings == nil {
		writeErr(w, 503, "recordings store not initialized")
		return
	}
	id := r.PathValue("id")
	rid := r.URL.Query().Get("recorder_id")
	meta, err := s.recordings.Get(rid, id)
	if err != nil {
		if errors.Is(err, recordings.ErrNotFound) {
			writeErr(w, 404, "not found")
			return
		}
		writeErr(w, 400, err.Error())
		return
	}
	path, err := s.recordings.AudioPath(rid, meta.ID)
	if err != nil {
		writeErr(w, 404, err.Error())
		return
	}
	w.Header().Set("Content-Type", meta.AudioMIME)
	w.Header().Set("Cache-Control", "private, max-age=300")
	http.ServeFile(w, r, path)
}

// recordingsDelete: DELETE /recordings/{id}?recorder_id=...
func (s *Server) recordingsDelete(w http.ResponseWriter, r *http.Request) {
	if s.recordings == nil {
		writeErr(w, 503, "recordings store not initialized")
		return
	}
	id := r.PathValue("id")
	rid := r.URL.Query().Get("recorder_id")
	if err := recordings.SafeID(rid); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if err := recordings.SafeID(id); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	// Confirm the recording belongs to this recorder before delete — Get
	// returns ErrNotFound when the meta isn't there, which protects against
	// guessing IDs across recorder_ids.
	if _, err := s.recordings.Get(rid, id); err != nil {
		if errors.Is(err, recordings.ErrNotFound) {
			writeErr(w, 404, "not found")
			return
		}
		writeErr(w, 400, err.Error())
		return
	}
	if err := s.recordings.Delete(rid, id); err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

// recordingsDeleteAll: DELETE /recordings?recorder_id=...
func (s *Server) recordingsDeleteAll(w http.ResponseWriter, r *http.Request) {
	if s.recordings == nil {
		writeErr(w, 503, "recordings store not initialized")
		return
	}
	rid := r.URL.Query().Get("recorder_id")
	if err := recordings.SafeID(rid); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	n, err := s.recordings.DeleteAll(rid)
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true, "deleted": n})
}

