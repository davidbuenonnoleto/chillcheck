-- Migration 0006: corrective-action logging (HACCP documentation for deviations)

CREATE TABLE IF NOT EXISTS corrective_actions (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id      uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    alert_id    uuid NOT NULL REFERENCES alerts(id) ON DELETE CASCADE,
    action      text NOT NULL,                          -- adjusted_equipment | relocated_product | discarded_product | other
    disposition text NOT NULL DEFAULT 'not_affected',   -- not_affected | relocated | discarded
    note        text NOT NULL DEFAULT '',
    recorded_by uuid REFERENCES users(id) ON DELETE SET NULL,
    created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_corrective_actions_alert ON corrective_actions(alert_id);
CREATE INDEX IF NOT EXISTS idx_corrective_actions_org ON corrective_actions(org_id);
