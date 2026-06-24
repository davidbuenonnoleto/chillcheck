-- Migration 0005: team invites + password reset

CREATE TABLE IF NOT EXISTS invites (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id      uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    email       text NOT NULL,
    role        text NOT NULL DEFAULT 'staff',     -- 'admin' | 'staff'
    token_hash  text NOT NULL UNIQUE,              -- sha256 of the invite token
    invited_by  uuid REFERENCES users(id) ON DELETE SET NULL,
    expires_at  timestamptz NOT NULL,
    accepted_at timestamptz,
    created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_invites_org ON invites(org_id);

CREATE TABLE IF NOT EXISTS password_resets (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash  text NOT NULL UNIQUE,
    expires_at  timestamptz NOT NULL,
    used_at     timestamptz,
    created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_password_resets_user ON password_resets(user_id);
