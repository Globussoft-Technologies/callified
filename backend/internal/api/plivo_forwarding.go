package api

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/globussoft/callified-backend/internal/db"
)

func forwardingActionOutcome(status, bleg string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "completed":
		if bleg == "" {
			return "unknown", true
		}
		return "answered", true
	case "busy":
		return "busy", true
	case "no-answer", "timeout":
		return "no_answer", true
	case "failed":
		return "failed", true
	case "cancel", "canceled", "cancelled":
		return "cancelled", true
	default:
		return "", false
	}
}

// Called only after Plivo V3 verification: the signed URL and carrier A-leg
// identity must identify the same account-scoped incoming call.
func plivoForwardContext(account *db.OrgExotelAccount, query, form url.Values) (string, error) {
	if account == nil || account.ID <= 0 || account.OrgID <= 0 || account.Provider != "plivo" || len(query["context_id"]) != 1 {
		return "", fmt.Errorf("invalid forwarding context")
	}
	callID := form.Get("DialALegUUID")
	if callID == "" {
		callID = form.Get("CallUUID")
	}
	contextID := query.Get("context_id")
	if callID == "" || len(callID) > 80 || contextID != fmt.Sprintf("plivo_in_%d_%s", account.ID, callID) {
		return "", fmt.Errorf("invalid forwarding context")
	}
	if len(form.Get("DialBLegUUID")) > 80 {
		return "", fmt.Errorf("invalid B-leg identity")
	}
	return contextID, nil
}

func (s *Server) plivoForwardCallback(w http.ResponseWriter, r *http.Request, action bool) {
	account, ok := s.verifyPlivo(w, r)
	if !ok {
		return
	}
	contextID, err := plivoForwardContext(account, r.URL.Query(), r.PostForm)
	if err != nil {
		http.Error(w, "Invalid forwarding context", http.StatusForbidden)
		return
	}
	route, err := s.db.GetIncomingRouteSnapshot(r.Context(), account.OrgID, contextID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		http.Error(w, "Forwarding context unavailable", http.StatusServiceUnavailable)
		return
	}
	if err != nil || route.Mode != "phone" {
		http.Error(w, "Unknown forwarding call", http.StatusConflict)
		return
	}
	if to := r.PostForm.Get("DialBLegTo"); to != "" && plivoNumber(to) != plivoNumber(route.Phone) {
		http.Error(w, "Forwarding destination mismatch", http.StatusForbidden)
		return
	}
	bleg := r.PostForm.Get("DialBLegUUID")
	outcome := ""
	if action {
		var valid bool
		outcome, valid = forwardingActionOutcome(r.PostForm.Get("DialStatus"), bleg)
		if !valid {
			http.Error(w, "Invalid dial status", http.StatusBadRequest)
			return
		}
	} else {
		switch strings.ToLower(r.PostForm.Get("DialAction")) {
		case "answer", "connected":
			if bleg == "" {
				http.Error(w, "Missing B-leg identity", http.StatusBadRequest)
				return
			}
			outcome = "answered"
		case "hangup":
			status := r.PostForm.Get("DialBLegStatus")
			if strings.EqualFold(status, "completed") {
				// A hangup alone is not evidence that the destination answered.
				seconds, _ := strconv.ParseInt(r.PostForm.Get("DialBLegBillDuration"), 10, 64)
				if seconds > 0 && bleg != "" {
					outcome = "answered"
				}
			} else {
				outcome, _ = forwardingActionOutcome(status, bleg)
			}
		}
	}
	if outcome != "" {
		if err := s.db.SetForwardingOutcome(r.Context(), account.OrgID, contextID, outcome, bleg, action); err != nil {
			http.Error(w, "Forwarding status unavailable", http.StatusServiceUnavailable)
			return
		}
	}
	if action {
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte("<?xml version=\"1.0\" encoding=\"UTF-8\"?><Response><Hangup/></Response>"))
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (s *Server) plivoInboundDialAction(w http.ResponseWriter, r *http.Request) {
	s.plivoForwardCallback(w, r, true)
}
func (s *Server) plivoInboundDialEvent(w http.ResponseWriter, r *http.Request) {
	s.plivoForwardCallback(w, r, false)
}
