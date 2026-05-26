package api

import (
	"encoding/json"
	"net/http"

	"github.com/globussoft/callified-backend/internal/db"
)

// ── GET /api/api-keys ─────────────────────────────────────────────────────────

// @Summary     List API keys
// @Description Returns all API keys for the org (prefix shown, secret never returned). Requires Admin role.
// @Tags        api-keys
// @Produce     json
// @Security    BearerAuth
// @Success     200  {array}   db.APIKey
// @Failure     401  {object}  ErrorResponse
// @Failure     403  {object}  ErrorResponse
// @Failure     500  {object}  ErrorResponse
// @Router      /api/api-keys [get]
func (s *Server) listAPIKeys(w http.ResponseWriter, r *http.Request) {
	ac := getAuth(r)
	keys, err := s.db.GetAPIKeysByOrg(ac.OrgID)
	if err != nil {
		s.logger.Sugar().Errorw("listAPIKeys", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, emptyJSON(keys))
}

// ── POST /api/api-keys ────────────────────────────────────────────────────────

// @Summary     Create API key
// @Description Generates a new API key. The raw key is returned once — store it securely. Requires Admin role.
// @Tags        api-keys
// @Accept      json
// @Produce     json
// @Security    BearerAuth
// @Param       body  body      object{name=string}  true  "Key name/label"
// @Success     201   {object}  object{id=int64,key=string}
// @Failure     400   {object}  ErrorResponse
// @Failure     401   {object}  ErrorResponse
// @Failure     403   {object}  ErrorResponse
// @Failure     500   {object}  ErrorResponse
// @Router      /api/api-keys [post]
func (s *Server) createAPIKey(w http.ResponseWriter, r *http.Request) {
	ac := getAuth(r)
	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Name == "" {
		writeError(w, http.StatusBadRequest, "name required")
		return
	}
	raw, hashed, err := db.GenerateAPIKey()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "key generation failed")
		return
	}
	prefix := raw
	if len(raw) > 10 {
		prefix = raw[:10]
	}
	id, err := s.db.CreateAPIKey(ac.OrgID, body.Name, hashed, prefix)
	if err != nil {
		s.logger.Sugar().Errorw("createAPIKey", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	// Return the raw key once — it is never stored in plaintext
	writeJSON(w, http.StatusCreated, map[string]any{
		"id":  id,
		"key": raw,
	})
}

// ── DELETE /api/api-keys/{id} ─────────────────────────────────────────────────

// @Summary     Delete API key
// @Description Revokes an API key. Requires Admin role.
// @Tags        api-keys
// @Produce     json
// @Security    BearerAuth
// @Param       id  path      int64  true  "API Key ID"
// @Success     200  {object}  DeletedResponse
// @Failure     400  {object}  ErrorResponse
// @Failure     401  {object}  ErrorResponse
// @Failure     403  {object}  ErrorResponse
// @Failure     404  {object}  ErrorResponse
// @Failure     500  {object}  ErrorResponse
// @Router      /api/api-keys/{id} [delete]
func (s *Server) deleteAPIKey(w http.ResponseWriter, r *http.Request) {
	ac := getAuth(r)
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	deleted, err := s.db.DeleteAPIKey(ac.OrgID, id)
	if err != nil {
		s.logger.Sugar().Errorw("deleteAPIKey", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if !deleted {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"deleted": true})
}
