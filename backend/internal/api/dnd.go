package api

import (
	"encoding/csv"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/globussoft/callified-backend/internal/db"
)

// ── GET /api/dnd ──────────────────────────────────────────────────────────────

func (s *Server) listDND(w http.ResponseWriter, r *http.Request) {
	ac := getAuth(r)
	numbers, err := s.db.GetDNDNumbers(ac.OrgID)
	if err != nil {
		s.logger.Sugar().Errorw("listDND", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	// Frontend reads `{numbers, total}`; a bare array fell through the
	// `data.numbers || data.items || data.data || []` fallback chain and
	// always rendered as empty. Wrap + include a total so the page's "(N
	// total)" header matches what's actually returned. Pagination params
	// (page/per_page) are accepted but currently ignored — the DND list
	// is rarely large enough to need server-side pagination.
	if numbers == nil {
		numbers = []db.DNDNumber{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"numbers": numbers,
		"total":   len(numbers),
	})
}

// ── POST /api/dnd ─────────────────────────────────────────────────────────────

func (s *Server) addDND(w http.ResponseWriter, r *http.Request) {
	ac := getAuth(r)
	var body struct {
		Phone  string `json:"phone"`
		Source string `json:"source"`
		Reason string `json:"reason"` // legacy alias
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Phone == "" {
		writeError(w, http.StatusBadRequest, "phone required")
		return
	}
	src := body.Source
	if src == "" {
		src = body.Reason
	}
	if err := s.db.AddDNDNumber(ac.OrgID, body.Phone, src); err != nil {
		s.logger.Sugar().Errorw("addDND", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]bool{"added": true})
}

// ── POST /api/dnd/import-csv ──────────────────────────────────────────────────

func (s *Server) importDNDCSV(w http.ResponseWriter, r *http.Request) {
	ac := getAuth(r)
	if err := r.ParseMultipartForm(5 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "invalid multipart form")
		return
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "file required")
		return
	}
	defer file.Close()

	reader := csv.NewReader(file)
	records, err := reader.ReadAll()
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid CSV")
		return
	}

	var phones []string
	for i, rec := range records {
		if i == 0 {
			continue // skip header row
		}
		if len(rec) > 0 && strings.TrimSpace(rec[0]) != "" {
			phones = append(phones, strings.TrimSpace(rec[0]))
		}
	}

	if err := s.db.AddDNDNumbersBulk(ac.OrgID, phones, "manual"); err != nil {
		s.logger.Sugar().Errorw("importDNDCSV", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"imported": len(phones)})
}

// ── DELETE /api/dnd/{id} ──────────────────────────────────────────────────────
// Accepts either a numeric row ID or a phone string as the path segment.
// The frontend's DndPage.jsx Remove button passes the phone (it doesn't
// track row IDs client-side) and Python's API also keyed off phone — with
// a strict ID-only handler every Remove click 400'd.
func (s *Server) removeDND(w http.ResponseWriter, r *http.Request) {
	ac := getAuth(r)
	raw := r.PathValue("id")
	if raw == "" {
		writeError(w, http.StatusBadRequest, "id or phone required")
		return
	}
	// If it parses as a positive int, treat it as the row ID; otherwise
	// fall through to a phone match.
	if id, err := parseID(r, "id"); err == nil {
		deleted, dbErr := s.db.RemoveDNDNumber(ac.OrgID, id)
		if dbErr != nil {
			s.logger.Sugar().Errorw("removeDND/byID", "err", dbErr)
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		if deleted {
			writeJSON(w, http.StatusOK, map[string]bool{"deleted": true})
			return
		}
	}
	deleted, err := s.db.RemoveDNDNumberByPhone(ac.OrgID, raw)
	if err != nil {
		s.logger.Sugar().Errorw("removeDND/byPhone", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if !deleted {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"deleted": true})
}

// ── GET /api/dnd/check ────────────────────────────────────────────────────────

func (s *Server) checkDND(w http.ResponseWriter, r *http.Request) {
	ac := getAuth(r)
	phone := r.URL.Query().Get("phone")
	if phone == "" {
		writeError(w, http.StatusBadRequest, "phone query param required")
		return
	}
	isDND, err := s.db.IsDNDNumber(ac.OrgID, phone)
	if err != nil {
		s.logger.Sugar().Errorw("checkDND", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"is_dnd": isDND})
}

// ── GET /api/dnd/check/{phone} ────────────────────────────────────────────────
// Path-param flavour the frontend's Check button uses. Same return shape as
// the query-param version above.
func (s *Server) checkDNDByPhone(w http.ResponseWriter, r *http.Request) {
	ac := getAuth(r)
	phone := r.PathValue("phone")
	if phone == "" {
		writeError(w, http.StatusBadRequest, "phone required")
		return
	}
	isDND, err := s.db.IsDNDNumber(ac.OrgID, phone)
	if err != nil {
		s.logger.Sugar().Errorw("checkDNDByPhone", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"is_dnd": isDND})
}

// ── DELETE /api/dnd/phone/{phone} ─────────────────────────────────────────────
// Frontend Remove button sends the phone, not the row ID (which it doesn't
// have client-side). Delete by phone, scoped to the caller's org.
func (s *Server) removeDNDByPhone(w http.ResponseWriter, r *http.Request) {
	ac := getAuth(r)
	phone := r.PathValue("phone")
	if phone == "" {
		writeError(w, http.StatusBadRequest, "phone required")
		return
	}
	deleted, err := s.db.RemoveDNDNumberByPhone(ac.OrgID, phone)
	if err != nil {
		s.logger.Sugar().Errorw("removeDNDByPhone", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if !deleted {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"deleted": true})
}
