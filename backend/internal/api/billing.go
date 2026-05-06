package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"go.uber.org/zap"

	"github.com/globussoft/callified-backend/internal/billing"
)

// GET /api/billing/plans  (no auth — public)
func (s *Server) listBillingPlans(w http.ResponseWriter, r *http.Request) {
	plans, err := s.db.GetBillingPlans()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, emptyJSON(plans))
}

// GET /api/billing/subscription
func (s *Server) getSubscription(w http.ResponseWriter, r *http.Request) {
	ac := getAuth(r)
	sub, err := s.db.GetSubscriptionByOrg(ac.OrgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if sub == nil {
		// Return a sentinel so the frontend gets a valid object with status "none"
		writeJSON(w, http.StatusOK, map[string]string{"status": "none"})
		return
	}
	writeJSON(w, http.StatusOK, sub)
}

// GET /api/billing/usage
func (s *Server) getBillingUsage(w http.ResponseWriter, r *http.Request) {
	ac := getAuth(r)
	usage, err := s.db.GetBillingUsage(ac.OrgID)
	if err != nil {
		s.logger.Sugar().Errorw("getBillingUsage", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, usage)
}

// POST /api/billing/subscribe  (no Razorpay — direct activation for testing)
func (s *Server) billingSubscribe(w http.ResponseWriter, r *http.Request) {
	ac := getAuth(r)
	var body struct {
		PlanID int64 `json:"plan_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.PlanID == 0 {
		writeError(w, http.StatusBadRequest, "plan_id required")
		return
	}
	if _, err := s.db.CreateSubscription(ac.OrgID, body.PlanID, "monthly"); err != nil {
		s.logger.Sugar().Errorw("billingSubscribe", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	// Companion invoice — without this the Billing page's Invoices section
	// stays empty for an active plan, which looks broken to the user.
	// Razorpay-verified path creates one in billing.VerifyAndActivate; this
	// is the dev/test parallel.
	if plan, perr := s.db.GetBillingPlanByID(body.PlanID); perr == nil && plan != nil {
		invNum := fmt.Sprintf("INV-%d-%d", time.Now().Unix(), ac.OrgID)
		if _, err := s.db.CreateInvoice(ac.OrgID, invNum, "", "INR", float64(plan.PricePaise)/100); err != nil {
			s.logger.Sugar().Warnw("billingSubscribe: CreateInvoice failed", "err", err)
		}
	}
	writeJSON(w, http.StatusOK, map[string]bool{"subscribed": true})
}

// POST /api/billing/subscription
func (s *Server) createSubscription(w http.ResponseWriter, r *http.Request) {
	ac := getAuth(r)
	var body struct {
		PlanID       int64  `json:"plan_id"`
		BillingCycle string `json:"billing_cycle"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.PlanID == 0 {
		writeError(w, http.StatusBadRequest, "plan_id required")
		return
	}
	if body.BillingCycle == "" {
		body.BillingCycle = "monthly"
	}

	orderID, err := s.billingSvc.CreateOrder(r.Context(), ac.OrgID, body.PlanID, body.BillingCycle)
	if err != nil {
		s.logger.Warn("createSubscription: CreateOrder", zap.Error(err))
		writeError(w, http.StatusBadGateway, "failed to create order")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"order_id":      orderID,
		"razorpay_key":  s.cfg.RazorpayKeyID,
		"billing_cycle": body.BillingCycle,
	})
}

// DELETE /api/billing/subscription
func (s *Server) cancelSubscription(w http.ResponseWriter, r *http.Request) {
	ac := getAuth(r)
	if err := s.db.CancelSubscription(ac.OrgID); err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"cancelled": true})
}

// POST /api/billing/cancel  (frontend calls POST, not DELETE)
func (s *Server) cancelBillingPost(w http.ResponseWriter, r *http.Request) {
	s.cancelSubscription(w, r)
}

// POST /api/billing/create-order
func (s *Server) createOrder(w http.ResponseWriter, r *http.Request) {
	s.createSubscription(w, r) // same handler
}

// POST /api/billing/verify-payment
func (s *Server) verifyPayment(w http.ResponseWriter, r *http.Request) {
	ac := getAuth(r)
	var body struct {
		OrderID      string `json:"razorpay_order_id"`
		PaymentID    string `json:"razorpay_payment_id"`
		Signature    string `json:"razorpay_signature"`
		BillingCycle string `json:"billing_cycle"`
		PlanID       int64  `json:"plan_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if body.BillingCycle == "" {
		body.BillingCycle = "monthly"
	}

	invoiceNumber, err := s.billingSvc.VerifyAndActivate(
		r.Context(), ac.OrgID, body.PlanID,
		body.OrderID, body.PaymentID, body.Signature, body.BillingCycle)
	if err != nil {
		s.logger.Warn("verifyPayment failed", zap.Error(err))
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"invoice_number": invoiceNumber})
}

// GET /api/billing/payments
func (s *Server) listPayments(w http.ResponseWriter, r *http.Request) {
	ac := getAuth(r)
	payments, err := s.db.GetPaymentsByOrg(ac.OrgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, emptyJSON(payments))
}

// GET /api/billing/invoices
func (s *Server) listInvoices(w http.ResponseWriter, r *http.Request) {
	ac := getAuth(r)
	invoices, err := s.db.GetInvoicesByOrg(ac.OrgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, emptyJSON(invoices))
}

// GET /api/billing/invoices/{number}/download
func (s *Server) downloadInvoice(w http.ResponseWriter, r *http.Request) {
	ac := getAuth(r)
	invoiceNumber := r.PathValue("number")
	inv, err := s.db.GetInvoiceByNumber(ac.OrgID, invoiceNumber)
	if err != nil || inv == nil {
		writeError(w, http.StatusNotFound, "invoice not found")
		return
	}

	data := billing.InvoiceData{
		InvoiceNumber: inv.InvoiceNumber,
		Date:          firstDate(inv.CreatedAt),
		Status:        capitalizeStatus(inv.Status),
		OrgName:       "Your Organization",
		PlanName:      "Subscription",
		PaymentID:     inv.InvoiceNumber, // we don't track a separate payment id in the dev path
	}
	if org, err := s.db.GetOrganizationByID(ac.OrgID); err == nil && org != nil {
		data.OrgName = org.Name
	}
	if sub, err := s.db.GetSubscriptionByOrg(ac.OrgID); err == nil && sub != nil {
		data.PlanName = sub.PlanName
		data.PeriodStart = firstDate(sub.PeriodStart)
		data.PeriodEnd = firstDate(sub.PeriodEnd)
	}
	data.Subtotal, data.GST, data.Total = billing.NewInvoiceData(inv.AmountPaise)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	// `inline` lets the frontend render the HTML in an iframe (View flow);
	// `attachment` forced a download every time even when the user just
	// wanted to look at the invoice. The browser's "Save As…" / Print
	// menu still works on inline-rendered pages.
	w.Header().Set("Content-Disposition",
		fmt.Sprintf("inline; filename=invoice_%s.html", invoiceNumber))
	_, _ = io.WriteString(w, billing.GenerateInvoiceHTML(data))
}

// firstDate returns just the YYYY-MM-DD prefix of a "YYYY-MM-DD HH:MM:SS"
// timestamp. Empty input returns empty (template hides empty fields).
func firstDate(ts string) string {
	if len(ts) >= 10 {
		return ts[:10]
	}
	return ts
}

// capitalizeStatus turns "paid" → "Paid" so the template's badge styles match.
func capitalizeStatus(s string) string {
	if s == "" {
		return ""
	}
	return string(s[0]-32) + s[1:]
}

// POST /api/billing/webhook  (no auth — HMAC-verified)
func (s *Server) razorpayWebhook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		w.WriteHeader(http.StatusOK)
		return
	}

	sig := r.Header.Get("X-Razorpay-Signature")
	if sig != "" && s.cfg.RazorpayWebhookSecret != "" {
		if !billing.VerifyWebhookSignature(s.cfg.RazorpayWebhookSecret, body, sig) {
			writeError(w, http.StatusUnauthorized, "invalid signature")
			return
		}
	}

	var event struct {
		Event   string `json:"event"`
		Payload struct {
			Payment struct {
				Entity struct {
					ID      string `json:"id"`
					OrderID string `json:"order_id"`
					Amount  int64  `json:"amount"`
				} `json:"entity"`
			} `json:"payment"`
		} `json:"payload"`
	}
	if err := json.Unmarshal(body, &event); err != nil {
		w.WriteHeader(http.StatusOK)
		return
	}

	if event.Event == "payment.captured" {
		orderID := event.Payload.Payment.Entity.OrderID
		paymentID := event.Payload.Payment.Entity.ID

		payment, _ := s.db.GetPaymentByOrderID(orderID)
		if payment != nil && payment.Status == "created" {
			if err := s.db.CompleteRazorpayPayment(orderID, paymentID); err != nil {
				s.logger.Warn("razorpayWebhook: CompleteRazorpayPayment", zap.Error(err))
			}
			// plan_id must come from Razorpay order notes — skip subscription creation here;
			// VerifyAndActivate (called client-side) handles it with plan_id from the request.
		}
	}

	w.WriteHeader(http.StatusOK)
}
