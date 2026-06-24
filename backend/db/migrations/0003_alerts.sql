-- Migration 0003: alert engine (out-of-range / overdue notifications)
--   psql "$DATABASE_URL" -f db/migrations/0003_alerts.sql

CREATE TABLE IF NOT EXISTS alerts (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id      uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    unit_id     uuid NOT NULL REFERENCES units(id) ON DELETE CASCADE,
    kind        text NOT NULL,                    -- 'out_of_range' | 'overdue'
    status      text NOT NULL DEFAULT 'open',     -- 'open' | 'resolved'
    detail      text,
    opened_at   timestamptz NOT NULL DEFAULT now(),
    notified_at timestamptz,
    resolved_at timestamptz
);

-- At most one open alert per unit per kind (prevents duplicate notifications).
CREATE UNIQUE INDEX IF NOT EXISTS idx_alerts_open
    ON alerts(unit_id, kind) WHERE status = 'open';
CREATE INDEX IF NOT EXISTS idx_alerts_org_time ON alerts(org_id, opened_at DESC);
