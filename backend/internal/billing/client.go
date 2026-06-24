package billing

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/stripe/stripe-go/v79"
	bpsession "github.com/stripe/stripe-go/v79/billingportal/session"
	cosession "github.com/stripe/stripe-go/v79/checkout/session"
	"github.com/stripe/stripe-go/v79/customer"
	"github.com/stripe/stripe-go/v79/subscription"
	"github.com/stripe/stripe-go/v79/subscriptionitem"
	"github.com/stripe/stripe-go/v79/webhook"
)

// Client wraps the Stripe SDK for the few operations ChillCheck needs. We use
// hosted Checkout and the Billing Portal, so no card data ever touches our servers.
type Client struct {
	priceID       string
	webhookSecret string
}

func NewClient(secretKey, priceID, webhookSecret string) *Client {
	stripe.Key = secretKey
	return &Client{priceID: priceID, webhookSecret: webhookSecret}
}

func (c *Client) CreateCustomer(orgID, orgName, email string) (string, error) {
	params := &stripe.CustomerParams{
		Name:  stripe.String(orgName),
		Email: stripe.String(email),
	}
	params.AddMetadata("org_id", orgID)
	cust, err := customer.New(params)
	if err != nil {
		return "", err
	}
	return cust.ID, nil
}

func (c *Client) CheckoutURL(customerID, successURL, cancelURL string, quantity int64) (string, error) {
	if quantity < 1 {
		quantity = 1
	}
	params := &stripe.CheckoutSessionParams{
		Mode:       stripe.String(string(stripe.CheckoutSessionModeSubscription)),
		Customer:   stripe.String(customerID),
		SuccessURL: stripe.String(successURL),
		CancelURL:  stripe.String(cancelURL),
		LineItems: []*stripe.CheckoutSessionLineItemParams{{
			Price:    stripe.String(c.priceID),
			Quantity: stripe.Int64(quantity),
		}},
	}
	sess, err := cosession.New(params)
	if err != nil {
		return "", err
	}
	return sess.URL, nil
}

// SetSubscriptionQuantity updates the per-location quantity on the subscription's
// first line item (used when locations are added).
func (c *Client) SetSubscriptionQuantity(subID string, quantity int64) error {
	if quantity < 1 {
		quantity = 1
	}
	sub, err := subscription.Get(subID, nil)
	if err != nil {
		return err
	}
	if len(sub.Items.Data) == 0 {
		return fmt.Errorf("subscription %s has no items", subID)
	}
	_, err = subscriptionitem.Update(sub.Items.Data[0].ID, &stripe.SubscriptionItemParams{
		Quantity: stripe.Int64(quantity),
	})
	return err
}

func (c *Client) PortalURL(customerID, returnURL string) (string, error) {
	params := &stripe.BillingPortalSessionParams{
		Customer:  stripe.String(customerID),
		ReturnURL: stripe.String(returnURL),
	}
	ps, err := bpsession.New(params)
	if err != nil {
		return "", err
	}
	return ps.URL, nil
}

// SubscriptionUpdate is the normalized view of a subscription webhook event.
type SubscriptionUpdate struct {
	CustomerID string
	SubID      string
	Status     string
	Plan       string
	PeriodEnd  *time.Time
}

// ParseSubscriptionEvent verifies the webhook signature and, for subscription
// lifecycle events, returns a normalized update. ok is false for events we ignore.
func (c *Client) ParseSubscriptionEvent(payload []byte, sigHeader string) (SubscriptionUpdate, bool, error) {
	event, err := webhook.ConstructEvent(payload, sigHeader, c.webhookSecret)
	if err != nil {
		return SubscriptionUpdate{}, false, err
	}

	switch event.Type {
	case "customer.subscription.created", "customer.subscription.updated", "customer.subscription.deleted":
		var sub stripe.Subscription
		if err := json.Unmarshal(event.Data.Raw, &sub); err != nil {
			return SubscriptionUpdate{}, false, err
		}
		upd := SubscriptionUpdate{
			CustomerID: sub.Customer.ID,
			SubID:      sub.ID,
			Status:     string(sub.Status),
		}
		if event.Type == "customer.subscription.deleted" {
			upd.Status = "canceled"
		}
		if sub.CurrentPeriodEnd > 0 {
			t := time.Unix(sub.CurrentPeriodEnd, 0)
			upd.PeriodEnd = &t
		}
		if len(sub.Items.Data) > 0 && sub.Items.Data[0].Price != nil {
			if upd.Plan = sub.Items.Data[0].Price.Nickname; upd.Plan == "" {
				upd.Plan = sub.Items.Data[0].Price.ID
			}
		}
		return upd, true, nil
	}
	return SubscriptionUpdate{}, false, nil
}
