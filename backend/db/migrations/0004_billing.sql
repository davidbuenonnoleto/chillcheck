-- Migration 0004: Stripe billing (Weeks 7+)
--   psql "$DATABASE_URL" -f db/migrations/0004_billing.sql

ALTER TABLE organizations
    ADD COLUMN IF NOT EXISTS stripe_customer_id     text,
    ADD COLUMN IF NOT EXISTS stripe_subscription_id text,
    ADD COLUMN IF NOT EXISTS plan                   text,
    ADD COLUMN IF NOT EXISTS subscription_status    text NOT NULL DEFAULT 'trialing',
    ADD COLUMN IF NOT EXISTS trial_end              timestamptz,
    ADD COLUMN IF NOT EXISTS current_period_end     timestamptz;

CREATE UNIQUE INDEX IF NOT EXISTS idx_orgs_stripe_customer
    ON organizations(stripe_customer_id) WHERE stripe_customer_id IS NOT NULL;

-- Give existing orgs a 14-day trial so they aren't immediately gated.
UPDATE organizations SET trial_end = now() + interval '14 days' WHERE trial_end IS NULL;
