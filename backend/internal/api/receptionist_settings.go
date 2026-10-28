package api

import (
	"encoding/json"
	"github.com/globussoft/callified-backend/internal/db"
	"io"
	"net/http"
	"strconv"
	"strings"
)

func (s *Server) receptionistSettings(w http.ResponseWriter, r *http.Request) {
	orgID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || orgID <= 0 {
		writeError(w, 400, "invalid organization")
		return
	}
	if !s.authorizeOrg(w, r, orgID) {
		return
	}
	if s.db == nil {
		writeError(w, 503, "database unavailable")
		return
	}
	if r.Method == http.MethodGet {
		settings, err := s.db.GetReceptionistSettings(orgID)
		if err != nil {
			writeError(w, 500, "could not load instructions")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(settings)
		return
	}
	var settings db.ReceptionistSettings
	r.Body = http.MaxBytesReader(w, r.Body, 96000)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if decoder.Decode(&settings) != nil {
		writeError(w, 400, "invalid settings")
		return
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		writeError(w, 400, "invalid settings")
		return
	}
	settings.Greeting = strings.TrimSpace(settings.Greeting)
	settings.Instructions = strings.TrimSpace(settings.Instructions)
	if settings.Greeting == "" || settings.Instructions == "" || len(settings.Greeting) > 4000 || len(settings.Instructions) > 20000 {
		writeError(w, 400, "greeting and instructions are required (maximum 4000 and 20000 bytes)")
		return
	}
	if err := s.db.SaveReceptionistSettings(orgID, settings); err != nil {
		writeError(w, 500, "could not save instructions")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(settings)
}
