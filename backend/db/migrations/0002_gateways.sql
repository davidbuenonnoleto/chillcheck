-- Migration 0002: BLE gateway support (Weeks 4-6)
-- Apply to an existing DB:
--   psql "$DATABASE_URL" -f db/migrations/0002_gateways.sql

CREATE TABLE IF NOT EXISTS gateways (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id       uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    location_id  uuid NOT NULL REFERENCES locations(id) ON DELETE CASCADE,
    name         text NOT NULL,
    api_key_hash text NOT NULL UNIQUE,   -- sha256 hex of the gateway key
    key_prefix   text NOT NULL,          -- first chars of the key, for display only
    last_seen_at timestamptz,
    created_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_gateways_org ON gateways(org_id);

-- One BLE sensor maps to one unit. Stored uppercase, colon-separated (A4:C1:38:..).
ALTER TABLE units ADD COLUMN IF NOT EXISTS sensor_mac text;
CREATE UNIQUE INDEX IF NOT EXISTS idx_units_org_mac
    ON units(org_id, sensor_mac) WHERE sensor_mac IS NOT NULL;
