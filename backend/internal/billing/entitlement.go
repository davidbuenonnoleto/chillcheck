package billing

import (
	"time"

	"chillcheck/internal/store"
)

// Entitled reports whether an org currently has access to paid features. An
// active subscription, an unexpired trial, or a past_due subscription (Stripe is
// still retrying payment) all count; a canceled subscription or an expired trial
// without a subscription does not.
func Entitled(org store.Organization, now time.Time) bool {
	switch org.SubscriptionStatus {
	case "active":
		return true
	case "trialing":
		return org.TrialEnd == nil || org.TrialEnd.After(now)
	case "past_due":
		return true // lenient: show a banner, don't lock out mid-dunning
	default: // canceled, unpaid, incomplete, incomplete_expired
		return false
	}
}
