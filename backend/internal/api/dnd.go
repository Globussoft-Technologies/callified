package api

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/globussoft/callified-backend/internal/db"
)

// ── GET /api/dnd ──────────────────────────────────────────────────────────────

// @Summary     List DND numbers
// @Description Returns all Do-Not-Dial numbers for the org. Requires Admin role.
// @Tags        dnd
// @Produce     json
// @Security    BearerAuth
// @Success     200  {object}  object{numbers=[]db.DNDNumber,total=int}
// @Failure     401  {object}  ErrorResponse
// @Failure     403  {object}  ErrorResponse
// @Failure     500  {object}  ErrorResponse
// @Router      /api/dnd [get]
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

// @Summary     Add DND number
// @Description Adds a 10-digit phone number to the org's Do-Not-Dial list. Requires Admin role.
// @Tags        dnd
// @Accept      json
// @Produce     json
// @Security    BearerAuth
// @Param       body  body      object{phone=string,source=string}  true  "Phone number and optional source"
// @Success     201   {object}  BoolResponse
// @Failure     400   {object}  ErrorResponse
// @Failure     401   {object}  ErrorResponse
// @Failure     403   {object}  ErrorResponse
// @Failure     500   {object}  ErrorResponse
// @Router      /api/dnd [post]
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
	body.Phone = strings.TrimSpace(body.Phone)
	if !isValidPhone(body.Phone) {
		writeError(w, http.StatusBadRequest, "phone must be exactly 10 digits")
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

// @Summary     Import DND numbers from CSV
// @Description Bulk-adds phone numbers from a CSV file (first column = phone). Requires Admin role.
// @Tags        dnd
// @Accept      multipart/form-data
// @Produce     json
// @Security    BearerAuth
// @Param       file  formData  file  true  "CSV file (first column: 10-digit phone)"
// @Success     200   {object}  object{imported=int,skipped=[]string}
// @Failure     400   {object}  ErrorResponse
// @Failure     401   {object}  ErrorResponse
// @Failure     403   {object}  ErrorResponse
// @Failure     500   {object}  ErrorResponse
// @Router      /api/dnd/import-csv [post]
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
	var skipped []string
	for i, rec := range records {
		if i == 0 {
			continue // skip header row
		}
		if len(rec) == 0 {
			continue
		}
		p := strings.TrimSpace(rec[0])
		if p == "" {
			continue
		}
		if !isValidPhone(p) {
			skipped = append(skipped, fmt.Sprintf("row %d: %q not 10 digits", i+1, p))
			continue
		}
		phones = append(phones, p)
	}

	if err := s.db.AddDNDNumbersBulk(ac.OrgID, phones, "manual"); err != nil {
		s.logger.Sugar().Errorw("importDNDCSV", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"imported": len(phones), "skipped": skipped})
}

// ── DELETE /api/dnd/{id} ──────────────────────────────────────────────────────
// Accepts either a numeric row ID or a phone string as the path segment.
// The frontend's DndPage.jsx Remove button passes the phone (it doesn't
// track row IDs client-side) and Python's API also keyed off phone — with
// a strict ID-only handler every Remove click 400'd.
// @Summary     Remove DND entry
// @Description Removes a DND entry by row ID or phone number. Requires Admin role.
// @Tags        dnd
// @Produce     json
// @Security    BearerAuth
// @Param       id  path      string  true  "Row ID or 10-digit phone number"
// @Success     200  {object}  DeletedResponse
// @Failure     400  {object}  ErrorResponse
// @Failure     401  {object}  ErrorResponse
// @Failure     403  {object}  ErrorResponse
// @Failure     404  {object}  ErrorResponse
// @Failure     500  {object}  ErrorResponse
// @Router      /api/dnd/{id} [delete]
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

// @Summary     Check DND (query param)
// @Description Checks if a phone number is on the org's DND list.
// @Tags        dnd
// @Produce     json
// @Security    BearerAuth
// @Param       phone  query  string  true  "10-digit phone number"
// @Success     200  {object}  object{is_dnd=bool}
// @Failure     400  {object}  ErrorResponse
// @Failure     401  {object}  ErrorResponse
// @Failure     500  {object}  ErrorResponse
// @Router      /api/dnd/check [get]
func (s *Server) checkDND(w http.ResponseWriter, r *http.Request) {
	ac := getAuth(r)
	phone := strings.TrimSpace(r.URL.Query().Get("phone"))
	if phone == "" {
		writeError(w, http.StatusBadRequest, "phone query param required")
		return
	}
	if !isValidPhone(phone) {
		writeError(w, http.StatusBadRequest, "phone must be exactly 10 digits")
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
// @Summary     Check DND by phone (path param)
// @Description Checks if a phone number is on the org's DND list using path parameter.
// @Tags        dnd
// @Produce     json
// @Security    BearerAuth
// @Param       phone  path  string  true  "10-digit phone number"
// @Success     200  {object}  object{is_dnd=bool}
// @Failure     400  {object}  ErrorResponse
// @Failure     401  {object}  ErrorResponse
// @Failure     500  {object}  ErrorResponse
// @Router      /api/dnd/check/{phone} [get]
func (s *Server) checkDNDByPhone(w http.ResponseWriter, r *http.Request) {
	ac := getAuth(r)
	phone := strings.TrimSpace(r.PathValue("phone"))
	if phone == "" {
		writeError(w, http.StatusBadRequest, "phone required")
		return
	}
	if !isValidPhone(phone) {
		writeError(w, http.StatusBadRequest, "phone must be exactly 10 digits")
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
