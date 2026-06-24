# CLAUDE.md — ChillCheck

Context for Claude Code working in this repo. Read this before making changes.

## What this is

ChillCheck is a temperature / cold-chain compliance app for restaurants. It replaces
paper temperature-log clipboards and produces inspection-ready PDF compliance reports.
This codebase is the full system: accounts with team invites and password reset, locations,
units, manual logging, a live status board, and PDF export (1–3); a standalone Go BLE
**gateway agent** for automatic sensor readings and an in-process **alert engine** (4–6);
and **Stripe billing** with per-location pricing and an onboarding checklist (7+). Remaining
roadmap: usage analytics, native mobile. See `ARCHITECTURE.md` for the full picture before
changing how the pieces fit together.

## Working agreement (read this)

- **Yolo mode.** Proceed without asking for confirmation on file edits, installs, builds,
  and local runs. Don't stop to ask permission for routine work.
- **Git repo.** Tracked with git and pushed to the private GitHub repo
  `davidbuenonnoleto/chillcheck` (`origin`, SSH, default branch `main`). Commit and push
  only when asked. Never commit secrets — `.env` files are gitignored; only `.env.example`
  is tracked. End commit messages with the `Co-Authored-By` trailer.
- Keep changes scoped and runnable. Prefer small, verifiable edits.

## Layout

```
chillcheck/
  docker-compose.yml      # local Postgres
  backend/                # Go API
    cmd/api/main.go        # entrypoint
    cmd/seed/main.go       # demo-data seeder (go run ./cmd/seed)
    db/schema.sql          # DDL — load this into the DB by hand
    internal/config        # env config
    internal/auth          # bcrypt + JWT
    internal/store         # pgx queries + models (all org-scoped)
    internal/api           # router, middleware, handlers, PDF report
  frontend/               # Vite + React + TS + Tailwind + shadcn/ui + TanStack Query
    src/lib/api.ts         # typed API client; TS types mirror Go JSON exactly
    src/auth               # AuthContext (token in localStorage)
    src/hooks/queries.ts   # TanStack Query hooks
    src/components/ui       # shadcn components (new-york style)
    src/pages              # LoginPage, DashboardPage, LocationPage
  gateway/                # Weeks 4-6: standalone Go BLE gateway agent (own module)
    main.go                # wiring, run loop, store-and-forward delivery
    internal/ble           # decoders (Xiaomi ATC/pvvx, BTHome v2) + scanner + simulator
    internal/{sampler,spool,client,config,reading}
    systemd/               # service unit for running on a Pi
```

## Run it

```bash
# 1. Postgres
docker compose up -d

# 2. Schema (once)
psql "postgres://chillcheck:chillcheck@localhost:5432/chillcheck?sslmode=disable" -f backend/db/schema.sql

# 3. Backend (http://localhost:8080)
cd backend && cp .env.example .env && go mod tidy && go run ./cmd/api

# 4. Frontend (http://localhost:5173)
cd frontend && cp .env.example .env && npm install && npm run dev
```

## Key decisions (don't silently reverse these)

- **Auth is email/password + JWT (HS256) + bcrypt**, not Cognito — so it runs with zero
  AWS setup for early pilots. The whole app only depends on `(UserID, OrgID)` pulled from
  the request context in `internal/api`, so swapping in Cognito later means replacing
  `internal/auth` + the `requireAuth` middleware and dropping `password_hash`. Nothing else
  should need to change.
- **Temperatures are stored in Fahrenheit** (`temp_f`, `min_temp_f`, `max_temp_f`). US
  restaurants and inspectors use °F; storing the recorded value verbatim avoids conversion
  bugs in a compliance record. Don't introduce silent C↔F conversion.
- **`units`** = a monitored fridge / freezer / hot-hold. Logged manually now; a BLE sensor
  attaches to a unit in Weeks 4–6. (This is the table earlier notes called "sensors".)
- **Multi-tenancy is mandatory.** Every query in `internal/store` is scoped by `org_id`.
  Any new query or endpoint must keep that scoping — never trust a client-supplied id
  without confirming it belongs to the caller's org.
- **Deploy target:** Go API as a container on **AWS App Runner** (not Lambda — avoids the
  Postgres connection-exhaustion trap), RDS/Aurora Postgres, React on Amplify Hosting.
  The `Dockerfile` already builds a small static image suitable for App Runner/ECS.

## Conventions

- Go: standard library + chi router + pgx; plain SQL (no ORM). Handlers return JSON via the
  `writeJSON` / `writeErr` helpers. Errors are user-facing and plain — see existing copy.
- TS: types in `src/lib/api.ts` are the single source of truth and **must match the Go JSON
  tags** (snake_case). If you change a Go struct tag, update the TS type in the same change.
- UI copy: sentence case, plain verbs, active voice. Buttons say what happens ("Log temp",
  "Open PDF"). Status labels live in `src/components/StatusBadge.tsx`.
- Frontend stack: React 19, Vite 8, TypeScript 6, **Tailwind v4** (CSS-first — there is no
  `tailwind.config.js`/`postcss.config.js`; the `@tailwindcss/vite` plugin runs it and the
  theme lives in `src/index.css` under `@theme inline`, mapping the ChillCheck HSL palette
  incl. the custom `ok`/`warn` status colors). shadcn UI primitives import from the unified
  `radix-ui` package (e.g. `import { Dialog as DialogPrimitive } from "radix-ui"`), not the
  per-component `@radix-ui/react-*` packages. Routing is React Router 7.
- Light/dark theme: `src/hooks/useTheme.ts` toggles the `.dark` class on `<html>` (persisted
  to localStorage, OS-default on first visit); `main.tsx` applies it pre-paint to avoid a
  flash; the header `ThemeToggle` flips it. Both palettes live in `src/index.css`.
- LocationPage has three tabs (Board · Readings · Alerts, local state). Temperature trend
  charts use `recharts` via `TempTrendChart`, which lazy-loads `TempTrendChartImpl` so
  recharts stays out of the main bundle — keep that split. Chart colors come from the
  `--chart-1..5` tokens in `index.css`; the safe range is shaded from `--ok`.
- Unit status logic lives in one place: `handleLocationStatus` in `internal/api/handlers.go`
  (`ok` / `out_of_range` / `overdue` / `no_data`).
- Security hardening (don't silently remove): unauthenticated credential endpoints
  (login/register/forgot/reset/invite-accept) are throttled per client IP by an in-memory
  `rateLimiter` (`internal/api/ratelimit.go`, 10/min); `securityHeaders` middleware sets
  CSP/nosniff/frame-deny on API responses; and the **frontend** ships a build-time CSP
  (`frontend/vite.config.ts`) whose `connect-src` is derived from `VITE_API_URL`. The CSP is
  the real mitigation for the JWT being held in localStorage — keep it tight if you add
  external script/style/connect origins.

## Tests

- `cd backend && go test ./...` — unit tests (stdlib `testing`, no third-party assert libs):
  `internal/store` covers `ReadingHash` determinism/field-sensitivity + `ComputeStatus`
  boundaries; `internal/api` covers the auth `rateLimiter`.
- Integration tests in `internal/store` (file `chain_integration_test.go`) need a real
  Postgres and are gated behind `TEST_DATABASE_URL` — they `t.Skip` when it's unset, so the
  default `go test ./...` needs no database. They prove the hash chain detects edits/deletes
  and that `WithLeaderLock` serializes. Keep new DB-backed tests behind the same env gate.
  Get a Postgres via `docker compose up -d`; with no Docker/sudo, use the rootless recipe
  `make test-integration` (script `backend/db/rootless-postgres.sh`, doc
  `backend/db/rootless-postgres.md`).
- No frontend test runner is set up yet; the gate is `tsc -b && vite build` (`npm run build`).

## Status model

A unit's status on the board:
- `out_of_range` — latest reading is below min or above max.
- `overdue` — last reading older than `log_interval_minutes`.
- `ok` — in range and logged recently.
- `no_data` — no readings yet.

## Gateway + ingest (Weeks 4–6, built)

- Sensors are bound to units by MAC (`units.sensor_mac`, uppercase colon form). A
  **gateway** row holds an API key — only its sha256 hash is stored (`auth/gateway.go`),
  looked up per request (fast, not bcrypt). Plaintext key is returned exactly once at
  creation.
- Ingest is a separate auth path: `requireGateway` middleware reads `X-Gateway-Key`,
  resolves org+location, and serves `POST /api/ingest/readings`. Readings come in keyed by
  MAC; unknown MACs are ignored, known ones inserted with `source = 'sensor'`.
- **Sensor readings preserve the gateway's timestamp** (`CreateSensorReading`) so buffered
  offline data lands at its real measurement time. Future timestamps are clamped to now.
- The agent lives in `gateway/` and store-and-forwards to disk on outages — never reverse
  that buffering, it's what keeps the compliance record gap-free.
- **Build the gateway with the standard Go toolchain (`go build`), NOT TinyGo.**
  `tinygo.org/x/bluetooth` is just the import path — it's a normal Go module. On Linux it
  uses BlueZ over D-Bus, pure Go, so cross-compile with `CGO_ENABLED=0 GOOS=linux
  GOARCH=arm64`. The agent uses `net/http` + a filesystem spool, so it needs a real OS
  (Pi / mini-PC); don't try to target a microcontroller with TinyGo without redesigning it.

## Alert engine (built)

- Runs **in-process** in the API as a background goroutine (`internal/alerts`), evaluating
  every unit on `ALERT_INTERVAL` (default 1m). Started from `cmd/api/main.go` when
  `ALERTS_ENABLED`.
- Status comes from `store.ComputeStatus` — the **single source of truth** shared with the
  dashboard handler. Change status rules there only.
- Reconciliation: one open alert per `(unit, kind)` via a partial unique index. A new
  problem opens an alert and notifies once; recovery (or a kind change) resolves it. Kinds
  are `out_of_range` and `overdue` (not `no_data` — a never-seen unit isn't a failure yet).
- Email goes through one shared `internal/email.Mailer` (not the old alerts notifier):
  `LogMailer` by default (zero setup), `SMTPMailer` when `SMTP_HOST` is set. The alert
  engine, team invites, and password resets all use it. Twilio/SNS = new `Mailer` impls.
  `buildMailer` in `cmd/api/main.go` wires it once and passes it to both the engine and the
  API server.
- Recipients = admin users of the org (`AdminEmailsByOrg`). Read recent alerts via
  `GET /api/locations/{id}/alerts`.
- Multi-replica safe: each tick runs under `store.WithLeaderLock` (a Postgres
  `pg_try_advisory_lock`), so across multiple API replicas only the lock holder evaluates a
  tick and alerts aren't sent N times. Non-holders skip the tick. The lock is held on a
  dedicated pooled connection for the tick's duration and released after.

## Corrective actions (HACCP documentation)

- A `corrective_actions` row (migration 0006) attaches to an **alert** — alerts are the
  deviation grouping (one open per unit+kind), so actions document an incident, not each
  reading. Fields: `action` (adjusted_equipment | relocated_product | discarded_product |
  other), `disposition` (not_affected | relocated | discarded), free-text `note`,
  `recorded_by`, `created_at`. Validation sets live in `internal/api/corrective_actions.go`.
- Endpoints (authed): `GET`/`POST /api/alerts/{id}/corrective-actions`. Create validates the
  alert is org-scoped via `AlertByID` first. `ListRecentAlerts` carries a
  `corrective_action_count` so the UI can flag undocumented deviations.
- The **compliance PDF** is the payoff: `renderDeviations` adds a "Deviations & corrective
  actions" section listing each alert in the window with its actions, or a red "No corrective
  action recorded" — which is itself meaningful to an inspector. Don't drop that flag.
- Frontend: `CorrectiveActionDialog.tsx` (log + review), wired into `AlertsPanel.tsx`.

## Tamper-evident reading chain

- Every reading is a link in a **per-org hash chain** (`internal/store/chain.go`):
  `row_hash = sha256(chain_seq, org, unit, recorded_by, temp, source, recorded_at, prev_hash)`.
  Both manual and sensor inserts go through **one** path, `insertChainedReading`, which takes
  a `pg_advisory_xact_lock` per org, reads the latest `(chain_seq, row_hash)`, and appends.
  Don't add another reading INSERT that bypasses it.
- **Determinism is critical** — a false "tampering" alarm would be terrible. So temps are
  rounded once with `round1` (half away from zero, matching `numeric(5,1)`) and the rounded
  value is both stored and hashed; timestamps are truncated to microseconds (Postgres
  precision) and hashed as UTC RFC3339Nano. `ReadingHash` MUST be identical at insert and at
  verify — change one, change both.
- `VerifyReadingChain` walks `chain_seq` order, recomputes each hash, and reports the first
  break. `GET /api/integrity` exposes it; the compliance PDF prints a VERIFIED/FAILED line;
  the dashboard shows `IntegrityBadge`. Threat model: detecting edits/deletes made directly
  in the DB (the app exposes no reading update/delete).
- Only readings created after migration 0007 are chained. The **seed** chains its rows too
  (it calls `store.ReadingHash` and threads `chain_seq`/`prev_hash`), so a fresh seed verifies
  clean. A DB with pre-0007 readings would need a backfill (none exists yet — pre-launch).

## Billing (Stripe, built)

- **Disabled by default**: if `STRIPE_SECRET_KEY` is unset, `cfg.BillingEnabled()` is false,
  the `Server.billing` client is nil, entitlement is always true, and no gating happens. So
  local dev needs zero Stripe setup.
- One org = one Stripe customer (`organizations.stripe_customer_id`). Hosted **Checkout**
  (subscribe) and **Billing Portal** (manage), so no card data hits our servers.
- Subscription state is synced **only** via the webhook `POST /api/webhooks/stripe`
  (`internal/api/billing.go` → `store.UpdateOrgSubscription`, keyed by customer id). The
  webhook is a public route, verified by Stripe signature — never trust the client for
  status. New orgs get a 14-day trial set in `CreateOrgWithAdmin`.
- Entitlement (`internal/billing/entitlement.go`): active / unexpired-trial / past_due =
  entitled; canceled / expired-trial = not. `requireEntitled` gates **only** create-location
  and create-unit (expansion). Do not gate logging, ingest, reads, or exports — an unpaid
  account must never lose or be locked out of its compliance data.
- **Admin-only**: checkout and portal require `user.Role == "admin"` (403 otherwise). The
  frontend hides the buttons for non-admins. `GET /api/billing` stays readable by all so the
  banner shows for everyone.
- **Per-location pricing**: subscription quantity = location count. Set at checkout
  (`CountLocations`), and re-synced after a location is created via `syncBillingQuantity`
  (async, best-effort, no-op when billing off or no active sub → never blocks the user).
- **Onboarding**: `OnboardingCard.tsx` on the dashboard shows a checklist (add location →
  add unit → log/connect) and auto-hides once a location with a unit exists.
- SDK is `github.com/stripe/stripe-go/v79`; run `go get .../stripe-go/v79@latest` to pin the
  real latest patch. Frontend: `BillingPage.tsx`, `BillingBanner.tsx`, `useBilling`.

## Team invites & password reset

- Tables `invites` and `password_resets` (migration 0005). Both store only a **sha256 hash**
  of the token (`auth.RandomToken` / `auth.HashToken`); the raw token lives only in the
  link. Handlers in `internal/api/invites.go`.
- Invites are **admin-only** to create/list/revoke (`requireAdmin`). Create returns
  `accept_url` and also emails it, so it works whether or not SMTP is configured. Accept is
  **public** (`POST /api/invites/accept`): it creates the user with the invite's email+role
  and logs them in. Public lookup uses a query token (`GET /api/invites/lookup?token=`) to
  avoid a chi path-param conflict with `DELETE /api/invites/{id}`.
- Password reset is **public** and must not leak account existence: `POST /api/auth/forgot`
  always returns 200; `POST /api/auth/reset` consumes a single-use, 1-hour token. Frontend:
  `TeamPage.tsx`, `AcceptInvitePage.tsx`, `ForgotPasswordPage.tsx`, `ResetPasswordPage.tsx`;
  `AuthContext.adoptToken` logs a user in from the invite-accept token.

## Roadmap (not built yet)

- Usage analytics, native mobile.

## Caveats

- Safe-temperature thresholds vary by state/local health department. The defaults in
  `AddUnitDialog` (fridge 33–40, freezer −10–10, hot-hold 135–165 °F) are starting points,
  not legal guidance. Let operators set their own ranges.
