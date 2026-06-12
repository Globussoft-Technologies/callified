package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/globussoft/callified-backend/internal/db"
)

// AdminSubscriptionRequest is the payload for creating/updating a subscription.
type AdminSubscriptionRequest struct {
	AdminEmail string    `json:"admin_email"`
	ExpiresAt  time.Time `json:"expires_at"`
	Plan       string    `json:"plan,omitempty"`
	IsActive   bool      `json:"is_active,omitempty"`
}

// AdminSubscriptionResponse is the payload returned for a subscription.
type AdminSubscriptionResponse struct {
	AdminEmail string    `json:"admin_email"`
	ExpiresAt  time.Time `json:"expires_at"`
	Plan       string    `json:"plan"`
	IsActive   bool      `json:"is_active"`
	Status     string    `json:"status"`
}

// isSuperAdmin checks whether the given email is the configured super-admin
// or has the SuperAdmin role in the DB.
func (s *Server) isSuperAdmin(email string) bool {
	if email == "" {
		return false
	}
	if s.cfg.SuperAdminEmail != "" && email == s.cfg.SuperAdminEmail {
		return true
	}
	if s.db != nil {
		if u, err := s.db.GetUserByEmail(email); err == nil && u != nil && u.Role == "SuperAdmin" {
			return true
		}
	}
	return false
}

// requireSuperAdmin gates an endpoint behind the configured super-admin email
// or a user whose DB role is "SuperAdmin". It revalidates the role from the DB
// so role changes take effect immediately.
func (s *Server) requireSuperAdmin(next http.HandlerFunc) http.HandlerFunc {
	return s.requireAuth(func(w http.ResponseWriter, r *http.Request) {
		ac := getAuth(r)
		if s.isSuperAdmin(ac.Email) {
			next.ServeHTTP(w, r)
			return
		}
		writeError(w, http.StatusForbidden, "forbidden")
	})
}

// @Summary     Create or update subscription
// @Description Super-admin endpoint to set or extend a subscription for an admin email.
// @Tags        admin
// @Accept      json
// @Produce     json
// @Param       body  body      AdminSubscriptionRequest  true  "Subscription payload"
// @Success     200   {object}  AdminSubscriptionResponse
// @Failure     400   {object}  ErrorResponse
// @Failure     403   {object}  ErrorResponse
// @Failure     500   {object}  ErrorResponse
// @Router      /api/admin/subscriptions [post]
func (s *Server) createOrUpdateSubscription(w http.ResponseWriter, r *http.Request) {
	var req AdminSubscriptionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.AdminEmail == "" {
		writeError(w, http.StatusBadRequest, "admin_email is required")
		return
	}
	if req.ExpiresAt.IsZero() {
		writeError(w, http.StatusBadRequest, "expires_at is required")
		return
	}
	if req.Plan == "" {
		req.Plan = "standard"
	}

	if err := s.db.CreateOrUpdateAdminSubscription(req.AdminEmail, req.ExpiresAt.UTC(), req.Plan, req.IsActive); err != nil {
		s.logger.Sugar().Errorw("createOrUpdateSubscription failed", "err", err, "email", req.AdminEmail)
		writeError(w, http.StatusInternalServerError, "failed to save subscription")
		return
	}

	status, err := s.db.ValidateAdminSubscription(req.AdminEmail)
	if err != nil {
		s.logger.Sugar().Errorw("ValidateAdminSubscription after save failed", "err", err, "email", req.AdminEmail)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	statusText := "active"
	if !status.Active {
		if status.Expired {
			statusText = "expired"
		} else {
			statusText = "inactive"
		}
	}

	writeJSON(w, http.StatusOK, AdminSubscriptionResponse{
		AdminEmail: req.AdminEmail,
		ExpiresAt:  req.ExpiresAt.UTC(),
		Plan:       req.Plan,
		IsActive:   req.IsActive,
		Status:     statusText,
	})
}

// @Summary     Get subscription
// @Description Super-admin endpoint to fetch a subscription by admin email.
// @Tags        admin
// @Produce     json
// @Param       email  path      string  true  "Admin email"
// @Success     200    {object}  AdminSubscriptionResponse
// @Failure     404    {object}  ErrorResponse
// @Failure     403    {object}  ErrorResponse
// @Router      /api/admin/subscriptions/{email} [get]
func (s *Server) getAdminSubscription(w http.ResponseWriter, r *http.Request) {
	email := r.PathValue("email")
	if email == "" {
		writeError(w, http.StatusBadRequest, "email is required")
		return
	}

	sub, err := s.db.GetAdminSubscriptionByEmail(email)
	if err != nil {
		s.logger.Sugar().Errorw("getSubscription failed", "err", err, "email", email)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if sub == nil {
		writeError(w, http.StatusNotFound, "subscription not found")
		return
	}

	statusText := "active"
	now := time.Now().UTC()
	if !sub.IsActive {
		statusText = "inactive"
	} else if sub.ExpiresAt.Before(now) || sub.ExpiresAt.Equal(now) {
		statusText = "expired"
	}

	writeJSON(w, http.StatusOK, AdminSubscriptionResponse{
		AdminEmail: sub.AdminEmail,
		ExpiresAt:  sub.ExpiresAt,
		Plan:       sub.Plan,
		IsActive:   sub.IsActive,
		Status:     statusText,
	})
}

// subscriptionError is a structured error for subscription failures.
type subscriptionError struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	ExpiresAt string `json:"expires_at,omitempty"`
	Plan      string `json:"plan,omitempty"`
}

// checkSubscription validates the admin's subscription during login.
func (s *Server) checkSubscription(email string) (*subscriptionError, error) {
	status, err := s.db.ValidateAdminSubscription(email)
	if err != nil {
		return nil, err
	}
	if !status.Found {
		return &subscriptionError{
			Code:    "SUBSCRIPTION_NOT_FOUND",
			Message: "No active subscription found for this account. Please contact support to activate your subscription.",
		}, nil
	}
	if status.Expired {
		return &subscriptionError{
			Code:       "SUBSCRIPTION_EXPIRED",
			Message:    "Your subscription expired on " + status.ExpiresAt.UTC().Format("2006-01-02") + ". Please renew to continue.",
			ExpiresAt:  status.ExpiresAt.UTC().Format(time.RFC3339),
			Plan:       status.Plan,
		}, nil
	}
	if !status.Active {
		return &subscriptionError{
			Code:    "SUBSCRIPTION_INACTIVE",
			Message: "Your subscription is currently inactive. Please contact support.",
			Plan:    status.Plan,
		}, nil
	}
	return nil, nil
}

// writeSubscriptionError sends a 403 response with subscription error details.
func writeSubscriptionError(w http.ResponseWriter, err *subscriptionError) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusForbidden)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error":      err.Message,
		"code":       err.Code,
		"expires_at": err.ExpiresAt,
		"plan":       err.Plan,
	})
}

// Ensure db package usage is referenced.
var _ = db.AdminSubscription{}
