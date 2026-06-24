-- ChillCheck schema (Weeks 1-3 core: manual logging + compliance reports)
-- Run against an empty database, e.g.:  psql "$DATABASE_URL" -f db/schema.sql

CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE IF NOT EXISTS organizations (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name        text NOT NULL,
    stripe_customer_id     text,
    stripe_subscription_id text,
    plan                   text,
    subscription_status    text NOT NULL DEFAULT 'trialing',
    trial_end              timestamptz,
    current_period_end     timestamptz,
    created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_orgs_stripe_customer
    ON organizations(stripe_customer_id) WHERE stripe_customer_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS users (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id        uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    email         text NOT NULL UNIQUE,
    name          text NOT NULL,
    password_hash text NOT NULL,
    role          text NOT NULL DEFAULT 'staff', -- 'admin' | 'staff'
    created_at    timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_users_org ON users(org_id);

CREATE TABLE IF NOT EXISTS locations (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id      uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name        text NOT NULL,
    timezone    text NOT NULL DEFAULT 'America/Los_Angeles',
    created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_locations_org ON locations(org_id);

-- A "unit" is a monitored point: a fridge, freezer, or hot-holding station.
-- It is logged manually now; a Bluetooth sensor attaches to it in weeks 4-6.
CREATE TABLE IF NOT EXISTS units (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id              uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    location_id         uuid NOT NULL REFERENCES locations(id) ON DELETE CASCADE,
    name                text NOT NULL,
    kind                text NOT NULL DEFAULT 'fridge', -- 'fridge' | 'freezer' | 'hot_hold'
    min_temp_f          numeric(5,1) NOT NULL,
    max_temp_f          numeric(5,1) NOT NULL,
    log_interval_minutes integer NOT NULL DEFAULT 240,
    created_at          timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_units_location ON units(location_id);

CREATE TABLE IF NOT EXISTS readings (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id       uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    unit_id      uuid NOT NULL REFERENCES units(id) ON DELETE CASCADE,
    temp_f       numeric(5,1) NOT NULL,
    source       text NOT NULL DEFAULT 'manual', -- 'manual' | 'sensor'
    note         text,
    recorded_by  uuid REFERENCES users(id) ON DELETE SET NULL,
    recorded_at  timestamptz NOT NULL DEFAULT now(),
    chain_seq    bigint,    -- tamper-evident hash chain (per org)
    prev_hash    text,
    row_hash     text
);
CREATE INDEX IF NOT EXISTS idx_readings_unit_time ON readings(unit_id, recorded_at DESC);
CREATE INDEX IF NOT EXISTS idx_readings_org_time ON readings(org_id, recorded_at DESC);
CREATE UNIQUE INDEX IF NOT EXISTS idx_readings_chain ON readings(org_id, chain_seq);

-- A BLE gateway: a Go agent on-site (Raspberry Pi / mini-PC) that forwards sensor
-- readings. Authenticated by an API key (we store only its sha256 hash).
CREATE TABLE IF NOT EXISTS gateways (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id       uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    location_id  uuid NOT NULL REFERENCES locations(id) ON DELETE CASCADE,
    name         text NOT NULL,
    api_key_hash text NOT NULL UNIQUE,
    key_prefix   text NOT NULL,
    last_seen_at timestamptz,
    created_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_gateways_org ON gateways(org_id);

-- A unit may have one BLE sensor bound to it by MAC (uppercase, colon-separated).
ALTER TABLE units ADD COLUMN IF NOT EXISTS sensor_mac text;
CREATE UNIQUE INDEX IF NOT EXISTS idx_units_org_mac
    ON units(org_id, sensor_mac) WHERE sensor_mac IS NOT NULL;

-- Alerts raised by the background engine when a unit goes out of range or stops
-- reporting. At most one open alert per unit per kind.
CREATE TABLE IF NOT EXISTS alerts (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id      uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    unit_id     uuid NOT NULL REFERENCES units(id) ON DELETE CASCADE,
    kind        text NOT NULL,
    status      text NOT NULL DEFAULT 'open',
    detail      text,
    opened_at   timestamptz NOT NULL DEFAULT now(),
    notified_at timestamptz,
    resolved_at timestamptz
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_alerts_open
    ON alerts(unit_id, kind) WHERE status = 'open';
CREATE INDEX IF NOT EXISTS idx_alerts_org_time ON alerts(org_id, opened_at DESC);

-- Pending team invitations (staff/admin join links).
CREATE TABLE IF NOT EXISTS invites (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id      uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    email       text NOT NULL,
    role        text NOT NULL DEFAULT 'staff',
    token_hash  text NOT NULL UNIQUE,
    invited_by  uuid REFERENCES users(id) ON DELETE SET NULL,
    expires_at  timestamptz NOT NULL,
    accepted_at timestamptz,
    created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_invites_org ON invites(org_id);

-- Single-use password reset tokens.
CREATE TABLE IF NOT EXISTS password_resets (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash  text NOT NULL UNIQUE,
    expires_at  timestamptz NOT NULL,
    used_at     timestamptz,
    created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_password_resets_user ON password_resets(user_id);

-- Documented corrective actions for deviations (each attaches to an alert).
CREATE TABLE IF NOT EXISTS corrective_actions (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id      uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    alert_id    uuid NOT NULL REFERENCES alerts(id) ON DELETE CASCADE,
    action      text NOT NULL,
    disposition text NOT NULL DEFAULT 'not_affected',
    note        text NOT NULL DEFAULT '',
    recorded_by uuid REFERENCES users(id) ON DELETE SET NULL,
    created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_corrective_actions_alert ON corrective_actions(alert_id);
CREATE INDEX IF NOT EXISTS idx_corrective_actions_org ON corrective_actions(org_id);
