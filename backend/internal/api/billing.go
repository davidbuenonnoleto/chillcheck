package api

import (
	"context"
	"io"
	"log"
	"net/http"
	"time"

	"chillcheck/internal/billing"
	"chillcheck/internal/store"

	"github.com/google/uuid"
)

func (s *Server) isEntitled(org store.Organization) bool {
	if !s.cfg.BillingEnabled() {
		return true
	}
	return billing.Entitled(org, time.Now())
}

// requireEntitled gates an action behind an active subscription. It is a no-op
// when billing is not configured. Returns false (and writes 402) if blocked.
func (s *Server) requireEntitled(w http.ResponseWriter, r *http.Request) bool {
	if !s.cfg.BillingEnabled() {
		return true
	}
	org, err := s.store.OrgByID(r.Context(), orgID(r))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "could not check subscription")
		return false
	}
	if !billing.Entitled(org, time.Now()) {
		writeErr(w, http.StatusPaymentRequired, "your subscription is inactive — open Billing to continue")
		return false
	}
	return true
}

func (s *Server) ensureCustomer(ctx context.Context, org store.Organization, email string) (string, error) {
	if org.StripeCustomerID != nil && *org.StripeCustomerID != "" {
		return *org.StripeCustomerID, nil
	}
	custID, err := s.billing.CreateCustomer(org.ID.String(), org.Name, email)
	if err != nil {
		return "", err
	}
	if err := s.store.SetOrgStripeCustomer(ctx, org.ID, custID); err != nil {
		return "", err
	}
	return custID, nil
}

func (s *Server) handleBillingStatus(w http.ResponseWriter, r *http.Request) {
	org, err := s.store.OrgByID(r.Context(), orgID(r))
	if err != nil {
		writeErr(w, http.StatusNotFound, "organization not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"billing_enabled":    s.cfg.BillingEnabled(),
		"status":             org.SubscriptionStatus,
		"plan":               org.Plan,
		"trial_end":          org.TrialEnd,
		"current_period_end": org.CurrentPeriodEnd,
		"entitled":           s.isEntitled(org),
	})
}

func (s *Server) handleBillingCheckout(w http.ResponseWriter, r *http.Request) {
	if s.billing == nil {
		writeErr(w, http.StatusServiceUnavailable, "billing is not configured")
		return
	}
	org, err := s.store.OrgByID(r.Context(), orgID(r))
	if err != nil {
		writeErr(w, http.StatusNotFound, "organization not found")
		return
	}
	user, ok := s.requireAdmin(w, r)
	if !ok {
		return
	}
	custID, err := s.ensureCustomer(r.Context(), org, user.Email)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "could not reach Stripe")
		return
	}
	qty, _ := s.store.CountLocations(r.Context(), org.ID) // per-location pricing
	base := s.cfg.AppBaseURL
	url, err := s.billing.CheckoutURL(custID, base+"/billing?checkout=success", base+"/billing?checkout=cancel", int64(qty))
	if err != nil {
		writeErr(w, http.StatusBadGateway, "could not start checkout")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"url": url})
}

func (s *Server) handleBillingPortal(w http.ResponseWriter, r *http.Request) {
	if s.billing == nil {
		writeErr(w, http.StatusServiceUnavailable, "billing is not configured")
		return
	}
	org, err := s.store.OrgByID(r.Context(), orgID(r))
	if err != nil {
		writeErr(w, http.StatusNotFound, "organization not found")
		return
	}
	user, ok := s.requireAdmin(w, r)
	if !ok {
		return
	}
	custID, err := s.ensureCustomer(r.Context(), org, user.Email)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "could not reach Stripe")
		return
	}
	url, err := s.billing.PortalURL(custID, s.cfg.AppBaseURL+"/billing")
	if err != nil {
		writeErr(w, http.StatusBadGateway, "could not open billing portal")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"url": url})
}

// handleStripeWebhook receives subscription lifecycle events and syncs status.
// Public route: authenticated by Stripe's signature, not a user token.
func (s *Server) handleStripeWebhook(w http.ResponseWriter, r *http.Request) {
	if s.billing == nil {
		w.WriteHeader(http.StatusOK) // nothing to do
		return
	}
	payload, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "could not read body")
		return
	}
	upd, ok, err := s.billing.ParseSubscriptionEvent(payload, r.Header.Get("Stripe-Signature"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid webhook signature")
		return
	}
	if ok {
		if err := s.store.UpdateOrgSubscription(r.Context(), upd.CustomerID, upd.SubID, upd.Status, upd.Plan, upd.PeriodEnd); err != nil {
			log.Printf("billing webhook: update org: %v", err)
			writeErr(w, http.StatusInternalServerError, "could not record update")
			return
		}
	}
	w.WriteHeader(http.StatusOK)
}

// syncBillingQuantity updates Stripe's per-location quantity after a location is
// added. Best-effort and async: it never blocks or fails the user's action, and
// is a no-op when billing is off or the org has no active subscription.
func (s *Server) syncBillingQuantity(orgID uuid.UUID) {
	if s.billing == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	org, err := s.store.OrgByID(ctx, orgID)
	if err != nil || org.StripeSubscriptionID == nil || *org.StripeSubscriptionID == "" {
		return
	}
	switch org.SubscriptionStatus {
	case "active", "past_due", "trialing":
	default:
		return
	}
	n, err := s.store.CountLocations(ctx, orgID)
	if err != nil {
		log.Printf("billing: count locations: %v", err)
		return
	}
	if err := s.billing.SetSubscriptionQuantity(*org.StripeSubscriptionID, int64(n)); err != nil {
		log.Printf("billing: set quantity: %v", err)
	}
}
