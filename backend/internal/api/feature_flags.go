package api

import (
	"encoding/json"
	"net/http"

	"github.com/globussoft/callified-backend/internal/db"
)

// UserFeatureFlagRequest is the payload for setting a feature flag.
type UserFeatureFlagRequest struct {
	Email          string `json:"email"`
	HideAiFeatures bool   `json:"hide_ai_features"`
}

// UserFeatureFlagResponse is the payload returned for a feature flag.
type UserFeatureFlagResponse struct {
	Email          string `json:"email"`
	HideAiFeatures bool   `json:"hide_ai_features"`
}

// @Summary     Set user feature flag
// @Description Super-admin endpoint to enable/disable feature flags for an email.
// @Tags        admin
// @Accept      json
// @Produce     json
// @Param       body  body      UserFeatureFlagRequest  true  "Feature flag payload"
// @Success     200   {object}  UserFeatureFlagResponse
// @Failure     400   {object}  ErrorResponse
// @Failure     403   {object}  ErrorResponse
// @Failure     500   {object}  ErrorResponse
// @Router      /api/admin/feature-flags [post]
func (s *Server) setUserFeatureFlag(w http.ResponseWriter, r *http.Request) {
	var req UserFeatureFlagRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Email == "" {
		writeError(w, http.StatusBadRequest, "email is required")
		return
	}

	if err := s.db.SetUserFeatureFlag(req.Email, req.HideAiFeatures); err != nil {
		s.logger.Sugar().Errorw("setUserFeatureFlag failed", "err", err, "email", req.Email)
		writeError(w, http.StatusInternalServerError, "failed to save feature flag")
		return
	}

	writeJSON(w, http.StatusOK, UserFeatureFlagResponse{
		Email:          req.Email,
		HideAiFeatures: req.HideAiFeatures,
	})
}

// @Summary     Get user feature flag
// @Description Super-admin endpoint to fetch feature flags for an email.
// @Tags        admin
// @Produce     json
// @Param       email  path      string  true  "Email address"
// @Success     200    {object}  UserFeatureFlagResponse
// @Failure     404    {object}  ErrorResponse
// @Failure     403   {object}  ErrorResponse
// @Failure     500   {object}  ErrorResponse
// @Router      /api/admin/feature-flags/{email} [get]
func (s *Server) getUserFeatureFlag(w http.ResponseWriter, r *http.Request) {
	email := r.PathValue("email")
	if email == "" {
		writeError(w, http.StatusBadRequest, "email is required")
		return
	}

	flag, err := s.db.GetUserFeatureFlag(email)
	if err != nil {
		s.logger.Sugar().Errorw("getUserFeatureFlag failed", "err", err, "email", email)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if flag == nil {
		writeJSON(w, http.StatusOK, UserFeatureFlagResponse{
			Email:          email,
			HideAiFeatures: false,
		})
		return
	}

	writeJSON(w, http.StatusOK, UserFeatureFlagResponse{
		Email:          flag.Email,
		HideAiFeatures: flag.HideAiFeatures,
	})
}

// @Summary     Delete user feature flag
// @Description Super-admin endpoint to remove feature flags for an email.
// @Tags        admin
// @Produce     json
// @Param       email  path      string  true  "Email address"
// @Success     204    {string}  nil
// @Failure     400    {object}  ErrorResponse
// @Failure     403    {object}  ErrorResponse
// @Failure     500    {object}  ErrorResponse
// @Router      /api/admin/feature-flags/{email} [delete]
func (s *Server) deleteUserFeatureFlag(w http.ResponseWriter, r *http.Request) {
	email := r.PathValue("email")
	if email == "" {
		writeError(w, http.StatusBadRequest, "email is required")
		return
	}

	if err := s.db.DeleteUserFeatureFlag(email); err != nil {
		s.logger.Sugar().Errorw("deleteUserFeatureFlag failed", "err", err, "email", email)
		writeError(w, http.StatusInternalServerError, "failed to delete feature flag")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// Ensure db package usage is referenced.
var _ = db.UserFeatureFlag{}
