// Package billing handles Razorpay payments and subscription management.
package billing

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/globussoft/callified-backend/internal/db"
	"github.com/globussoft/callified-backend/internal/email"
)

// Service orchestrates billing operations.
type Service struct {
	db       *db.DB
	razorpay *RazorpayClient
	email    *email.Service
	log      *zap.Logger
}

// New creates a billing Service.
func New(database *db.DB, keyID, keySecret string, emailSvc *email.Service, log *zap.Logger) *Service {
	return &Service{
		db:       database,
		razorpay: newRazorpayClient(keyID, keySecret),
		email:    emailSvc,
		log:      log,
	}
}

// CreateOrder creates a Razorpay order for a plan purchase.
// Returns (orderID, error).
func (s *Service) CreateOrder(ctx context.Context, orgID, planID int64, billingCycle string) (string, error) {
	plans, err := s.db.GetBillingPlans()
	if err != nil {
		return "", err
	}
	var plan *db.BillingPlan
	for i := range plans {
		if plans[i].ID == planID {
			plan = &plans[i]
			break
		}
	}
	if plan == nil {
		return "", fmt.Errorf("plan %d not found", planID)
	}

	amountPaise := plan.PricePaise
	receipt := fmt.Sprintf("org%d-plan%d-%d", orgID, planID, time.Now().Unix())
	notes := map[string]string{
		"org_id":        fmt.Sprintf("%d", orgID),
		"plan_id":       fmt.Sprintf("%d", planID),
		"billing_cycle": billingCycle,
	}

	orderID, err := s.razorpay.CreateOrder(ctx, amountPaise, "INR", receipt, notes)
	if err != nil {
		return "", fmt.Errorf("razorpay CreateOrder: %w", err)
	}

	if _, err := s.db.CreateRazorpayOrder(orgID, planID, orderID, "INR", float64(amountPaise)/100); err != nil {
		s.log.Warn("billing: CreateRazorpayOrder DB failed", zap.Error(err))
	}

	return orderID, nil
}

// VerifyAndActivate verifies a payment signature and activates the subscription.
// planID must be passed from the API layer since the payment record no longer stores it.
// Returns the invoice number.
func (s *Service) VerifyAndActivate(ctx context.Context, orgID, planID int64, orderID, paymentID, signature, billingCycle string) (string, error) {
	if !s.razorpay.VerifySignature(orderID, paymentID, signature) {
		return "", fmt.Errorf("invalid payment signature")
	}

	payment, err := s.db.GetPaymentByOrderID(orderID)
	if err != nil || payment == nil {
		return "", fmt.Errorf("payment record not found for order %s", orderID)
	}

	if err := s.db.CompleteRazorpayPayment(orderID, paymentID); err != nil {
		s.log.Warn("billing: CompleteRazorpayPayment failed", zap.Error(err))
	}

	if _, err := s.db.CreateSubscription(orgID, planID, billingCycle); err != nil {
		s.log.Warn("billing: CreateSubscription failed", zap.Error(err))
	}

	invoiceNumber := fmt.Sprintf("INV-%d-%s", time.Now().Unix(), paymentID[:8])
	if _, err := s.db.CreateInvoice(orgID, invoiceNumber, paymentID, "INR", float64(payment.AmountPaise)/100); err != nil {
		s.log.Warn("billing: CreateInvoice failed", zap.Error(err))
	}

	return invoiceNumber, nil
}

// Razorpay returns the underlying Razorpay client (for webhook verification).
func (s *Service) Razorpay() *RazorpayClient { return s.razorpay }

// ── Credit top-up (prepaid pay-per-minute model) ─────────────────────────────
//
// Sits alongside the existing subscription flow rather than replacing it: orgs
// without a plan can buy credits at ₹5/min and pay only for what they use.
// The dialer (and post-call recording.Service) deducts from the balance per
// completed call.

// CreateCreditOrder creates a Razorpay order for a credit top-up of the given
// rupee amount. amountINR is whole rupees from the client (₹100, ₹500…) so we
// don't have to thread paise through the request body. Razorpay's minimum
// order value is ₹1, so any positive amount works.
func (s *Service) CreateCreditOrder(ctx context.Context, orgID int64, amountINR int64) (string, int64, error) {
	if amountINR <= 0 {
		return "", 0, fmt.Errorf("amount must be positive")
	}
	amountPaise := amountINR * 100
	receipt := fmt.Sprintf("org%d-credits-%d", orgID, time.Now().Unix())
	notes := map[string]string{
		"org_id": fmt.Sprintf("%d", orgID),
		"type":   "credit_topup",
	}
	orderID, err := s.razorpay.CreateOrder(ctx, amountPaise, "INR", receipt, notes)
	if err != nil {
		return "", 0, fmt.Errorf("razorpay CreateOrder: %w", err)
	}
	// Reuse billing_payments to keep the Payment History view unified — the
	// row has plan_id=0 since this is a credit purchase, not a subscription.
	if _, err := s.db.CreateRazorpayOrder(orgID, 0, orderID, "INR", float64(amountPaise)/100); err != nil {
		s.log.Warn("billing: CreateRazorpayOrder (credits) DB failed", zap.Error(err))
	}
	return orderID, amountPaise, nil
}

// VerifyAndAddCredits verifies the Razorpay payment signature, marks the
// payment captured, and credits the org's balance. Returns the new balance
// (paise) and the credit_transactions row ID for the audit trail.
func (s *Service) VerifyAndAddCredits(ctx context.Context, orgID int64, orderID, paymentID, signature string) (int64, error) {
	if !s.razorpay.VerifySignature(orderID, paymentID, signature) {
		return 0, fmt.Errorf("invalid payment signature")
	}
	payment, err := s.db.GetPaymentByOrderID(orderID)
	if err != nil || payment == nil {
		return 0, fmt.Errorf("payment record not found for order %s", orderID)
	}
	// Idempotency guard: already-captured payments don't credit twice.
	if payment.Status == "captured" {
		oc, _ := s.db.GetOrgCredit(orgID)
		if oc != nil {
			return oc.BalancePaise, nil
		}
		return 0, nil
	}
	if err := s.db.CompleteRazorpayPayment(orderID, paymentID); err != nil {
		s.log.Warn("billing: CompleteRazorpayPayment (credits) failed", zap.Error(err))
	}
	notes := fmt.Sprintf("Razorpay topup ₹%.2f", float64(payment.AmountPaise)/100)
	if _, err := s.db.AddCredits(orgID, payment.AmountPaise, "purchase", paymentID, notes); err != nil {
		return 0, fmt.Errorf("AddCredits: %w", err)
	}
	oc, _ := s.db.GetOrgCredit(orgID)
	if oc == nil {
		return 0, nil
	}
	return oc.BalancePaise, nil
}
