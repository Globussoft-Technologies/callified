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

// @Summary     Get credit balance
// @Description Returns the org's prepaid credit balance and minutes available. Requires Admin role.
// @Tags        billing
// @Produce     json
// @Security    BearerAuth
// @Success     200  {object}  db.OrgCredit
// @Failure     401  {object}  ErrorResponse
// @Failure     403  {object}  ErrorResponse
// @Failure     500  {object}  ErrorResponse
// @Router      /api/billing/credits [get]
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

// @Summary     Create credit top-up order
// @Description Creates a Razorpay order for topping up prepaid call credits (₹1 – ₹1,00,000). Requires Admin role.
// @Tags        billing
// @Accept      json
// @Produce     json
// @Security    BearerAuth
// @Param       body  body      object{amount_inr=int64}  true  "Amount in INR"
// @Success     200   {object}  object{order_id=string,amount=int64,currency=string,key_id=string}
// @Failure     400   {object}  ErrorResponse
// @Failure     401   {object}  ErrorResponse
// @Failure     403   {object}  ErrorResponse
// @Failure     502   {object}  ErrorResponse
// @Router      /api/billing/credits/topup [post]
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

// @Summary     Verify credit top-up
// @Description Verifies the Razorpay signature for a credit top-up and adds credits to the org balance. Requires Admin role.
// @Tags        billing
// @Accept      json
// @Produce     json
// @Security    BearerAuth
// @Param       body  body      object{razorpay_order_id=string,razorpay_payment_id=string,razorpay_signature=string}  true  "Payment verification"
// @Success     200   {object}  object{verified=bool,balance_paise=int64}
// @Failure     400   {object}  ErrorResponse
// @Failure     401   {object}  ErrorResponse
// @Failure     403   {object}  ErrorResponse
// @Router      /api/billing/credits/verify [post]
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

// @Summary     List credit transactions
// @Description Returns the last 50 credit ledger entries (purchases, deductions, refunds). Requires Admin role.
// @Tags        billing
// @Produce     json
// @Security    BearerAuth
// @Success     200  {array}   db.CreditTransaction
// @Failure     401  {object}  ErrorResponse
// @Failure     403  {object}  ErrorResponse
// @Failure     500  {object}  ErrorResponse
// @Router      /api/billing/credits/transactions [get]
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
