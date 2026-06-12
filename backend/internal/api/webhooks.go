package api

import (
	"encoding/json"
	"net/http"
)

// ── GET /api/webhooks ─────────────────────────────────────────────────────────

// @Summary     List webhooks
// @Description Returns all configured outbound webhooks for the org. Requires Admin role.
// @Tags        webhooks
// @Produce     json
// @Security    BearerAuth
// @Success     200  {array}   object
// @Failure     401  {object}  ErrorResponse
// @Failure     403  {object}  ErrorResponse
// @Failure     500  {object}  ErrorResponse
// @Router      /api/webhooks [get]
func (s *Server) listWebhooks(w http.ResponseWriter, r *http.Request) {
	ac := getAuth(r)
	hooks, err := s.db.GetWebhooksByOrg(ac.OrgID)
	if err != nil {
		s.logger.Sugar().Errorw("listWebhooks", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	// Never expose the secret key in list responses
	for i := range hooks {
		hooks[i].SecretKey = ""
	}
	writeJSON(w, http.StatusOK, emptyJSON(hooks))
}

// ── POST /api/webhooks ────────────────────────────────────────────────────────

// @Summary     Create webhook
// @Description Registers a new outbound webhook endpoint. Requires Admin role.
// @Tags        webhooks
// @Accept      json
// @Produce     json
// @Security    BearerAuth
// @Param       body  body      object{url=string,event=string,secret_key=string}  true  "Webhook config"
// @Success     201   {object}  IDResponse
// @Failure     400   {object}  ErrorResponse
// @Failure     401   {object}  ErrorResponse
// @Failure     403   {object}  ErrorResponse
// @Failure     500   {object}  ErrorResponse
// @Router      /api/webhooks [post]
func (s *Server) createWebhook(w http.ResponseWriter, r *http.Request) {
	ac := getAuth(r)
	var body struct {
		URL       string `json:"url"`
		Event     string `json:"event"`
		SecretKey string `json:"secret_key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.URL == "" || body.Event == "" {
		writeError(w, http.StatusBadRequest, "url and event required")
		return
	}
	id, err := s.db.CreateWebhook(ac.OrgID, body.URL, body.Event, body.SecretKey)
	if err != nil {
		s.logger.Sugar().Errorw("createWebhook", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]int64{"id": id})
}

// ── DELETE /api/webhooks/{id} ─────────────────────────────────────────────────

// @Summary     Delete webhook
// @Description Removes an outbound webhook. Requires Admin role.
// @Tags        webhooks
// @Produce     json
// @Security    BearerAuth
// @Param       id  path      int64  true  "Webhook ID"
// @Success     200  {object}  DeletedResponse
// @Failure     400  {object}  ErrorResponse
// @Failure     401  {object}  ErrorResponse
// @Failure     403  {object}  ErrorResponse
// @Failure     404  {object}  ErrorResponse
// @Failure     500  {object}  ErrorResponse
// @Router      /api/webhooks/{id} [delete]
func (s *Server) deleteWebhook(w http.ResponseWriter, r *http.Request) {
	ac := getAuth(r)
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	deleted, err := s.db.DeleteWebhook(ac.OrgID, id)
	if err != nil {
		s.logger.Sugar().Errorw("deleteWebhook", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if !deleted {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"deleted": true})
}

// ── GET /api/webhooks/{id}/logs ───────────────────────────────────────────────

// @Summary     Get webhook logs
// @Description Returns the last 50 delivery attempts for a webhook. Requires Admin role.
// @Tags        webhooks
// @Produce     json
// @Security    BearerAuth
// @Param       id  path      int64  true  "Webhook ID"
// @Success     200  {array}   object
// @Failure     400  {object}  ErrorResponse
// @Failure     401  {object}  ErrorResponse
// @Failure     403  {object}  ErrorResponse
// @Failure     500  {object}  ErrorResponse
// @Router      /api/webhooks/{id}/logs [get]
func (s *Server) getWebhookLogs(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	logs, err := s.db.GetWebhookLogs(id, 50)
	if err != nil {
		s.logger.Sugar().Errorw("getWebhookLogs", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, emptyJSON(logs))
}
