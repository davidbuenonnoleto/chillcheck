# Usage Analytics — Design Spec

**Date:** 2026-06-23
**Status:** Approved (pre-implementation)
**Topic:** Org-wide usage analytics dashboard for ChillCheck

## Summary

A new org-wide top-level page at `/analytics` that answers three questions on one
dashboard: compliance health over time, equipment/unit performance, and operational
accountability. It is a read-only view scoped to the caller's org, with a location
filter and a date-range preset selector.

This is the "usage analytics" item from the roadmap in `CLAUDE.md`.

## Goals

- Show how compliant the org is over time, where deviations cluster, and which units
  and gaps need attention — in one place.
- Stay exact and inspector-explainable: no statistical interpolation.
- Reuse existing patterns (org-scoped store queries, recharts lazy-load split,
  TanStack Query hooks, shadcn primitives).

## Non-goals (v1, YAGNI)

- Time-weighted "% time in range" (interpolating between samples). v1 uses
  reading-based percentage only.
- PDF integration. v1 exports CSV only; the compliance PDF is untouched.
- Custom arbitrary date-range picker. v1 ships presets (7 / 30 / 90 days) only.
- Per-location-timezone day bucketing. v1 buckets daily in UTC (documented caveat).
- Billing gate. Analytics is a read/export view and must never be gated (matches the
  rule in `CLAUDE.md`: never gate reads, ingest, logging, or exports).

## Core metric

**Compliance = % of readings in range** = `in-range readings / total readings`.

"In range" means a reading's `temp_f` is between that unit's own `min_temp_f` and
`max_temp_f` (inclusive). This is computed in SQL with no interpolation — exact,
deterministic, and easy to explain to an inspector.

## Scope & placement

- New **org-wide** top-level page at `/analytics`, linked from the header nav.
- Covers all locations by default; a **location filter** narrows to one location.
- A **date-range selector** with presets 7 / 30 / 90 days, defaulting to 30 days.
- All data is `org_id`-scoped from the request context. A supplied `location_id` is
  validated to belong to the caller's org before use (multi-tenancy is mandatory).

## Backend

### `backend/internal/store/analytics.go` (data layer)

`AnalyticsSummary(ctx, orgID uuid.UUID, from, to time.Time, locationID *uuid.UUID)`
returns a struct containing:

- **KPIs**
  - overall % in-range (across all matched readings)
  - total readings logged in window
  - total deviations: count of alerts opened in window
  - overdue events: count of `overdue`-kind alerts opened in window
  - undocumented deviations: count of `out_of_range` alerts in window with
    `corrective_action_count = 0`
- **Trend**: daily buckets `[{date, in_range, total, pct}]`, bucketed by
  `date_trunc('day', recorded_at)` in **UTC** (v1 caveat).
- **Per-unit rows**: `[{unit_id, unit_name, location_name, pct_in_range,
  total_readings, deviation_count, avg_temp_f, min_temp_f, max_temp_f,
  last_reading_at}]`. Here `min_temp_f`/`max_temp_f` are the observed reading
  extremes for the unit in the window (not the thresholds), for spotting drift.

Sketch of the per-unit aggregation (org-scoped, optional location filter):

```sql
SELECT u.id, u.name, l.name AS location_name,
       count(r.*)                                                          AS total,
       count(r.*) FILTER (WHERE r.temp_f BETWEEN u.min_temp_f AND u.max_temp_f) AS in_range,
       avg(r.temp_f), min(r.temp_f), max(r.temp_f), max(r.recorded_at)
FROM units u
JOIN locations l ON l.id = u.location_id
LEFT JOIN readings r ON r.unit_id = u.id
     AND r.recorded_at >= $2 AND r.recorded_at < $3
WHERE l.org_id = $1
  AND ($4::uuid IS NULL OR l.id = $4)
GROUP BY u.id, u.name, l.name;
```

Percentages are computed from the integer counts and rounded once for display.
Units with zero readings in the window appear with `total = 0` and a null/`n/a`
percentage rather than being dropped.

### `backend/internal/api/analytics.go` (HTTP layer)

Routes wired in `api.go`, behind `requireAuth` only (no `requireEntitled`):

- `GET /api/analytics?from=&to=&location_id=` → JSON summary. Parses and validates
  the range; missing range defaults to the last 30 days. Returns via `writeJSON`,
  errors via `writeErr` with plain user-facing copy.
- `GET /api/analytics/export.csv?from=&to=&location_id=` → CSV of the per-unit
  breakdown for the same window/filter. `Content-Type: text/csv` and a
  `Content-Disposition` filename including the date range.

## Frontend

- **`src/pages/AnalyticsPage.tsx`** — date-range preset selector + location filter
  (reusing the existing locations query), then: KPI cards → trend chart → per-unit
  table → CSV export button.
- **Widgets**
  - KPI summary cards and a sortable per-unit table built from shadcn primitives.
  - **Trend chart** following the existing recharts lazy-load split: an
    `AnalyticsTrendChart` wrapper that lazy-loads its impl so recharts stays out of
    the main bundle (mirrors `TempTrendChart`). Colors from `--chart-1..5`; the
    in-range/safe band shaded from `--ok`.
- **Types** added to `src/lib/api.ts`, mirroring the Go JSON tags exactly
  (snake_case) — single source of truth.
- **`useAnalytics` hook** in `src/hooks/queries.ts` (TanStack Query), keyed by
  `{from, to, locationID}`.
- **CSV export**: because the JWT lives in an `Authorization` header (not a cookie),
  the button fetches the CSV as a blob through the api client (with the auth header),
  then triggers a download via an object URL — not a plain `<a href>`.
- **Route + nav link** added to the React Router 7 setup and the header.

## Testing

- **Backend unit tests** for the pure math (pct/rounding, bucket assembly) in
  `internal/store`, no DB required (matches the stdlib-`testing`, no-assert-lib
  convention).
- **Backend DB-backed test** in `internal/store` gated behind `TEST_DATABASE_URL`
  (`t.Skip` when unset, same convention as `chain_integration_test.go`): verifies
  in-range counting, deviation counts, and that another org's data is excluded
  (org-scoping isolation).
- **Frontend**: gate stays `tsc -b && vite build` (no test runner yet).

## Conventions honored

- Multi-tenancy: every query `org_id`-scoped; `location_id` validated against org.
- Temperatures stay in Fahrenheit; no C↔F conversion.
- Reads/exports are never billing-gated.
- TS types mirror Go JSON tags; recharts stays lazy-loaded; status/range logic is not
  re-implemented divergently.

## Open caveat

Daily trend buckets are computed in UTC for v1. Locations carry their own timezone,
so an org spanning timezones (or near midnight) may see a reading attributed to an
adjacent calendar day versus its local day. Per-location-timezone bucketing is a
deliberate later refinement, not part of v1.
