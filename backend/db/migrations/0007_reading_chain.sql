-- Migration 0007: tamper-evident reading hash chain
--   Each reading carries a monotonic per-org chain_seq, the previous row's hash,
--   and its own row_hash = sha256(canonical fields + prev_hash). Editing or
--   deleting any past reading directly in the database breaks the chain, which
--   VerifyReadingChain detects.
--
--   Note: only readings created after this migration are chained. On a database
--   with pre-existing readings, regenerate (seed) or run a one-off backfill;
--   ChillCheck is pre-launch and local-only, so a fresh seed covers it.

ALTER TABLE readings
    ADD COLUMN IF NOT EXISTS chain_seq bigint,
    ADD COLUMN IF NOT EXISTS prev_hash text,
    ADD COLUMN IF NOT EXISTS row_hash  text;

CREATE UNIQUE INDEX IF NOT EXISTS idx_readings_chain ON readings(org_id, chain_seq);
