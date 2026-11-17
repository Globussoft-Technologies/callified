package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"unicode/utf8"
)

func (s *Server) manualCallNotes(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, "calls.transcripts") {
		return
	}
	if r.Method == http.MethodPatch && !s.requirePermission(w, r, "crm.edit") {
		return
	}
	t, ok := s.dispositionTranscript(w, r)
	if !ok {
		return
	}
	if !t.IsManualCall() {
		writeError(w, http.StatusBadRequest, "notes are available only for manual calls")
		return
	}
	if r.Method == http.MethodGet {
		notes, err := s.db.GetManualCallNotes(t.OrgID, t.ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "could not load notes")
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"notes": notes})
		return
	}
	var body struct {
		Notes *string `json:"notes"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 32*1024)
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Notes == nil {
		writeError(w, http.StatusBadRequest, "notes are required")
		return
	}
	notes := strings.TrimSpace(*body.Notes)
	if utf8.RuneCountInString(notes) > 5000 {
		writeError(w, http.StatusBadRequest, "notes must be 5000 characters or fewer")
		return
	}
	if err := s.db.SaveManualCallNotes(t.OrgID, t.ID, getAuth(r).UserID, notes); err != nil {
		writeError(w, http.StatusInternalServerError, "could not save notes")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"notes": notes})
}
