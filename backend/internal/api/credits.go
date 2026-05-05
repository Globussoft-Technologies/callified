package api

import (
	"encoding/json"
	"net/http"

	"go.uber.org/zap"
)

// ── GET /api/billing/credits ─────────────────────────────────────────────────
//
// Returns the org's prepaid credit balance, the rate per minute (paise), and
// the derived minutes-available value the UI shows on the Billing page.

func (s *Server) getOrgCredits(w http.ResponseWriter, r *http.Request) {
	ac := getAuth(r)
	oc, err := s.db.GetOrgCredit(ac.OrgID)
	if err != nil {
		s.logger.Sugar().Errorw("getOrgCredits", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, oc)
}

// ── POST /api/billing/credits/topup ──────────────────────────────────────────
//
// Creates a Razorpay order for the given rupee amount and returns the order
// metadata the frontend hands to the Razorpay Checkout widget. Falls back to
// a "no Razorpay configured" response so dev environments can still test the
// UI flow without real keys (mirrors the existing subscription path).

func (s *Server) createCreditOrder(w http.ResponseWriter, r *http.Request) {
	ac := getAuth(r)
	var body struct {
		AmountINR int64 `json:"amount_inr"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.AmountINR <= 0 {
		writeError(w, http.StatusBadRequest, "amount_inr (positive integer) required")
		return
	}
	// Reasonable bounds — block typo'd ₹100,00,000 hits and unhelpfully tiny
	// orders that Razorpay rejects anyway.
	if body.AmountINR < 1 || body.AmountINR > 1_00_000 {
		writeError(w, http.StatusBadRequest, "amount must be between ₹1 and ₹100,000")
		return
	}

	orderID, amountPaise, err := s.billingSvc.CreateCreditOrder(r.Context(), ac.OrgID, body.AmountINR)
	if err != nil {
		s.logger.Warn("createCreditOrder", zap.Error(err))
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"order_id":     orderID,
		"amount":       amountPaise,
		"currency":     "INR",
		"key_id":       s.cfg.RazorpayKeyID,
		"description":  "Callified call credits top-up",
		"amount_inr":   body.AmountINR,
	})
}

// ── POST /api/billing/credits/verify ─────────────────────────────────────────
//
// Verifies the Razorpay payment signature and credits the org's balance.
// Idempotent on the order ID — handler-level + db-level both guard against
// double-credit if the frontend retries the verify request.

func (s *Server) verifyCreditTopup(w http.ResponseWriter, r *http.Request) {
	ac := getAuth(r)
	var body struct {
		OrderID   string `json:"razorpay_order_id"`
		PaymentID string `json:"razorpay_payment_id"`
		Signature string `json:"razorpay_signature"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil ||
		body.OrderID == "" || body.PaymentID == "" || body.Signature == "" {
		writeError(w, http.StatusBadRequest, "order_id, payment_id, and signature required")
		return
	}

	balancePaise, err := s.billingSvc.VerifyAndAddCredits(
		r.Context(), ac.OrgID, body.OrderID, body.PaymentID, body.Signature)
	if err != nil {
		s.logger.Warn("verifyCreditTopup", zap.Error(err))
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"verified":      true,
		"balance_paise": balancePaise,
	})
}

// ── GET /api/billing/credits/transactions ────────────────────────────────────
//
// Returns the most-recent ledger entries (purchases, deductions, refunds) so
// the Billing page can show a history table beneath the balance.

func (s *Server) listCreditTransactions(w http.ResponseWriter, r *http.Request) {
	ac := getAuth(r)
	list, err := s.db.GetCreditTransactions(ac.OrgID, 50)
	if err != nil {
		s.logger.Sugar().Errorw("listCreditTransactions", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, emptyJSON(list))
}
