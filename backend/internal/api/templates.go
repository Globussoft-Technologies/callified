package api

import (
	"encoding/json"
	"net/http"

	"github.com/globussoft/callified-backend/internal/prompt"
)

// ── GET /api/templates ───────────────────────────────────────────────────────

type listTemplatesResponse struct {
	Templates []prompt.Template `json:"templates"`
}

// @Summary     List prompt templates
// @Description Returns active prompt templates for the org plus global defaults.
// @Tags        templates
// @Produce     json
// @Security    BearerAuth
// @Success     200  {object}  listTemplatesResponse
// @Failure     401  {object}  ErrorResponse
// @Failure     403  {object}  ErrorResponse
// @Failure     500  {object}  ErrorResponse
// @Router      /api/templates [get]
func (s *Server) listTemplates(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, "products.view") {
		return
	}
	ac := getAuth(r)
	registry := prompt.NewRegistry(s.db)
	templates, err := registry.ListTemplates(r.Context(), ac.OrgID)
	if err != nil {
		s.logger.Sugar().Errorw("listTemplates", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, listTemplatesResponse{Templates: templates})
}

// ── GET /api/templates/{id} ────────────────────────────────────────────────────

// @Summary     Get a prompt template
// @Description Returns a single prompt template by ID.
// @Tags        templates
// @Produce     json
// @Security    BearerAuth
// @Param       id  path  int64  true  "Template ID"
// @Success     200  {object}  prompt.Template
// @Failure     400  {object}  ErrorResponse
// @Failure     401  {object}  ErrorResponse
// @Failure     403  {object}  ErrorResponse
// @Failure     404  {object}  ErrorResponse
// @Failure     500  {object}  ErrorResponse
// @Router      /api/templates/{id} [get]
func (s *Server) getTemplate(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, "products.view") {
		return
	}
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	registry := prompt.NewRegistry(s.db)
	t, err := registry.GetTemplateByID(r.Context(), id)
	if err != nil {
		s.logger.Sugar().Errorw("getTemplate", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if t == nil {
		writeError(w, http.StatusNotFound, "template not found")
		return
	}
	writeJSON(w, http.StatusOK, t)
}

// ── POST /api/templates ────────────────────────────────────────────────────────

type createTemplateRequest struct {
	Name         string `json:"name"`
	Language     string `json:"language"`
	TemplateType string `json:"template_type"`
	ScriptBody   string `json:"script_body"`
}

// @Summary     Create prompt template
// @Description Creates an org-scoped prompt template.
// @Tags        templates
// @Accept      json
// @Produce     json
// @Security    BearerAuth
// @Param       body  body      createTemplateRequest  true  "Template data"
// @Success     201   {object}  prompt.Template
// @Failure     400   {object}  ErrorResponse
// @Failure     401   {object}  ErrorResponse
// @Failure     403   {object}  ErrorResponse
// @Failure     500   {object}  ErrorResponse
// @Router      /api/templates [post]
func (s *Server) createTemplate(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, "products.manage") {
		return
	}
	ac := getAuth(r)
	var req createTemplateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" || req.ScriptBody == "" {
		writeError(w, http.StatusBadRequest, "name and script_body required")
		return
	}
	if req.Language == "" {
		req.Language = "en"
	}
	if req.TemplateType == "" {
		req.TemplateType = "voice"
	}
	registry := prompt.NewRegistry(s.db)
	t, err := registry.CreateTemplate(r.Context(), ac.OrgID, req.Name, req.Language, req.TemplateType, req.ScriptBody)
	if err != nil {
		s.logger.Sugar().Errorw("createTemplate", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusCreated, t)
}

// ── PUT /api/templates/{id} ──────────────────────────────────────────────────

type updateTemplateRequest struct {
	ScriptBody string `json:"script_body"`
}

// @Summary     Update prompt template
// @Description Creates a new version of the template with the given script body.
// @Tags        templates
// @Accept      json
// @Produce     json
// @Security    BearerAuth
// @Param       id    path  int64                  true  "Template ID"
// @Param       body  body  updateTemplateRequest  true  "Updated script body"
// @Success     200   {object}  prompt.Template
// @Failure     400   {object}  ErrorResponse
// @Failure     401   {object}  ErrorResponse
// @Failure     403   {object}  ErrorResponse
// @Failure     404   {object}  ErrorResponse
// @Failure     500   {object}  ErrorResponse
// @Router      /api/templates/{id} [put]
func (s *Server) updateTemplate(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, "products.manage") {
		return
	}
	ac := getAuth(r)
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var req updateTemplateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ScriptBody == "" {
		writeError(w, http.StatusBadRequest, "script_body required")
		return
	}
	registry := prompt.NewRegistry(s.db)
	t, err := registry.UpdateTemplate(r.Context(), id, ac.OrgID, req.ScriptBody)
	if err != nil {
		s.logger.Sugar().Errorw("updateTemplate", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, t)
}

// ── POST /api/templates/seed-panora ────────────────────────────────────────────

// @Summary     Seed Panora templates
// @Description Creates the default Panora qualification templates for the org.
// @Tags        templates
// @Produce     json
// @Security    BearerAuth
// @Success     200   {object}  object{seeded=bool}
// @Failure     401   {object}  ErrorResponse
// @Failure     403   {object}  ErrorResponse
// @Failure     500   {object}  ErrorResponse
// @Router      /api/templates/seed-panora [post]
func (s *Server) seedPanoraTemplates(w http.ResponseWriter, r *http.Request) {
	if !s.requirePermission(w, r, "products.manage") {
		return
	}
	ac := getAuth(r)
	registry := prompt.NewRegistry(s.db)
	if err := registry.SeedPanoraTemplates(r.Context(), ac.OrgID); err != nil {
		s.logger.Sugar().Errorw("seedPanoraTemplates", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"seeded": true})
}
