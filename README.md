# ChillCheck

Temperature / cold-chain compliance for restaurants. Replaces paper temp-log
clipboards and produces inspection-ready PDF compliance reports.

This is the full system (built through Weeks 1–7 plus a polish pass): accounts, locations,
monitored units, manual logging, a live status board, and PDF export (1–3); an on-site
Bluetooth **sensor gateway** for automatic readings and an **alert engine** for
out-of-range / overdue notifications (4–6); and **Stripe billing** with per-location
pricing, trials, and an onboarding checklist (7+). See `ARCHITECTURE.md` for how the
pieces fit together.

- **Backend:** Go (chi, pgx, JWT) with an in-process alert engine and Stripe billing — `backend/`
- **Frontend:** React + TypeScript + Vite + Tailwind + shadcn/ui + TanStack Query — `frontend/`
- **Gateway:** standalone Go BLE agent for a Raspberry Pi / mini-PC — `gateway/`
- **Database:** PostgreSQL
- **Built to deploy on:** AWS App Runner (API) + RDS Postgres + Amplify Hosting (frontend)

The frontend type-checks and production-builds, and the Go backend and gateway both compile
and `go vet` clean (Go 1.23+, verified on Go 1.26). `go.mod`/`go.sum` are committed, so a
plain `go build ./...` works after the first module download. There's a unit + integration
test suite — see **Tests** below.

## Run locally

Prerequisites: Docker, Go 1.23+, Node 20.19+ (or 22.12+) — required by Vite 8.

```bash
# 1. Start Postgres
docker compose up -d

# 2. Load the schema (once)
psql "postgres://chillcheck:chillcheck@localhost:5432/chillcheck?sslmode=disable" \
  -f backend/db/schema.sql

# 3. Backend  ->  http://localhost:8080
cd backend
go mod tidy
go run ./cmd/api

# 4. Frontend  ->  http://localhost:5173   (new terminal)
cd frontend
npm install
npm run dev
```

Open http://localhost:5173, create an account, add a location, add a unit
(fridge / freezer / hot-hold), and start logging temperatures. "Export report"
produces the compliance PDF.

Config comes from environment variables with sensible local defaults — no setup
needed to run as above. To override (DB URL, JWT secret, SMTP, Stripe, etc.), export
the vars or use a tool like [direnv](https://direnv.net/); see `backend/.env.example`
and `frontend/.env.example` for the full list.

### Run locally without Docker

No Docker (or no sudo)? Use the rootless Postgres helper, which downloads and runs a
private Postgres under your home dir on port **5433** and loads the schema for you. It
replaces steps 1–2 above; point the backend's `DATABASE_URL` at it and run the frontend
unchanged:

```bash
# 1+2. Rootless Postgres on :5433, schema loaded (downloads on first run)
make pg-up

# 3. Backend  ->  http://localhost:8080
cd backend && go mod tidy
DATABASE_URL="postgres://chillcheck:chillcheck@localhost:5433/chillcheck?sslmode=disable" \
  go run ./cmd/api

# 4. Frontend  ->  http://localhost:5173   (new terminal, same as above)
cd frontend && npm install && npm run dev
```

Seed demo data the same way: `cd backend && DATABASE_URL="…localhost:5433…" go run ./cmd/seed`.
Stop it with `make pg-down` (keeps data) or `make pg-nuke` (deletes everything). See
`backend/db/rootless-postgres.md` for details.

### Demo data (optional)

To skip manual setup and see a populated board, run the seeder after the schema is
loaded:

```bash
cd backend && go run ./cmd/seed
```

It creates a demo restaurant and signs you in with:

```
demo@chillcheck.app  /  chillcheck123
```

The seeded units deliberately show all four board statuses — Walk-in cooler (in range),
Reach-in fridge (out of range), Chest freezer (check overdue), and Hot line (no readings
yet) — and there's a few days of history so the compliance PDF has content. Re-running
the seeder resets the demo data.

## Tests

From the repo root (Make targets must run from there):

```bash
make test               # unit: reading hash chain, ComputeStatus, auth rate limiter (no database)
make test-integration   # spin up a database, then run all backend tests incl. DB-gated ones
```

`make test` is just `cd backend && go test ./...`.

Integration tests exercise the tamper-evident hash chain (insert → tamper → detect), the
alert-engine leader lock, and the analytics rollup against a **real Postgres**. They're gated
behind `TEST_DATABASE_URL` and skip when it's unset, so `go test ./...` stays green without a
database:

```bash
TEST_DATABASE_URL="postgres://chillcheck:chillcheck@localhost:5432/chillcheck?sslmode=disable" \
  go test ./internal/store/ -run Integration -v
```

`make test-integration` provides that Postgres for you. With Docker, point it at
`docker compose up -d`. With **no Docker and no sudo**, it falls back to a rootless Postgres
(downloaded and run entirely under your home dir) via `backend/db/rootless-postgres.sh` —
managed by `make pg-up` / `make pg-down` / `make pg-nuke` and documented in
`backend/db/rootless-postgres.md`. Run `make help` for all targets.

## What's in it

- **Auth** — register creates an organization plus its first (admin) user; email +
  password, JWT bearer tokens.
- **Locations** — one per restaurant/kitchen.
- **Units** — a monitored fridge, freezer, or hot-holding station, each with a safe
  min/max °F and an expected check interval.
- **Readings** — manual temperature entries with optional notes.
- **Status board** — per unit: latest temp and a status of in range / out of range /
  check overdue / no readings, auto-refreshing each minute.
- **Corrective-action logging** — every deviation (alert) can carry documented corrective
  actions: what was done (adjusted equipment / relocated / discarded / other), what happened
  to the product, a note, and who recorded it. The alerts feed flags deviations that still
  need documentation.
- **Tamper-evident records** — every reading is part of a per-organization hash chain
  (each row hashes its own fields plus the previous row's hash). Editing or deleting a past
  reading directly in the database breaks the chain, which `GET /api/integrity` detects; the
  dashboard shows a "records verified" badge and the PDF carries a verification statement.
- **Compliance PDF** — every reading in a date range with safe ranges and pass/fail, a
  **Deviations & corrective actions** section, and a record-integrity line ("VERIFIED — N
  readings, hash chain intact") — the part an inspector actually scrutinizes.
- **Analytics** — an org-wide dashboard (with a location filter and 7/30/90-day ranges)
  showing % of readings in range, deviations, overdue events, and undocumented deviations,
  a daily in-range trend chart, and a per-unit breakdown (avg/min/max temp, last reading)
  that exports to CSV.

## Engineering decisions (deliberate)

- **JWT + bcrypt, not Cognito (yet).** Runs with zero AWS setup so you can demo to a
  restaurant immediately. The app only depends on `(UserID, OrgID)` from the request
  context, so swapping in Cognito later is contained to `internal/auth` + the auth
  middleware.
- **Temperatures in Fahrenheit.** US restaurants and inspectors use °F; storing the
  recorded value verbatim avoids conversion error in a compliance record.
- **`units`, not `sensors`.** A unit is monitored manually now and gets a Bluetooth
  sensor attached in Weeks 4–6 without a schema change.
- **App Runner over Lambda** for the Go API — avoids the Lambda + Postgres
  connection-exhaustion trap. The `backend/Dockerfile` builds a small static image.

## Bluetooth gateway (Weeks 4–6, included)

The `gateway/` directory is a standalone Go agent for a Raspberry Pi / mini-PC that
listens to cheap BLE temperature sensors and forwards readings to the API, buffering
to disk during outages. It has a simulation mode so you can test the whole pipeline
with no hardware. It builds with the **standard Go toolchain** (the `tinygo.org/x/bluetooth`
dependency is a normal Go module, not the TinyGo compiler; on Linux it uses BlueZ over
D-Bus). See `gateway/README.md`. The backend ingest endpoint
(`POST /api/ingest/readings`, gateway-key authenticated) and sensor-to-unit binding
(`PUT /api/units/{id}/sensor`) are part of this.

## Alerts (included)

A background engine in the API evaluates every unit on a schedule (`ALERT_INTERVAL`,
default 1m) and raises an alert when a unit goes **out of range** or a reading is
**overdue**, resolving it automatically on recovery. It notifies the org's admins once
per problem (not every cycle). With no mail server configured it logs alerts; set
`SMTP_HOST` (SES SMTP / Mailgun / Gmail app password, STARTTLS on 587) to send email.
Recent alerts are available at `GET /api/locations/{id}/alerts`. Swapping in Twilio/SNS
is a new implementation of the shared `email.Mailer` interface.

## Billing (Stripe, optional)

Each organization is a Stripe customer. New orgs get a 14-day trial automatically (no
card). Subscribing uses **Stripe Checkout** and managing/canceling uses the **Billing
Portal** — both hosted, so no card data touches our servers. Pricing is **per location**:
the subscription quantity tracks the org's location count (set at checkout and updated
when a location is added). A webhook (`POST /api/webhooks/stripe`) syncs subscription
status into the org. Only **admins** can start checkout or open the portal. Entitlement
gates only *expansion* (creating new locations/units) when a trial lapses or a subscription
ends — logging, reads, ingest, and report export keep working so an unpaid account never
loses compliance data. New accounts see a **Get set up** onboarding checklist on the
dashboard until their first location and unit exist.

Leave `STRIPE_SECRET_KEY` blank to disable billing entirely (everything is free, no
gating). To enable it: set `STRIPE_SECRET_KEY`, `STRIPE_PRICE_ID` (a recurring per-unit
price), `STRIPE_WEBHOOK_SECRET`, and `APP_BASE_URL`, run migration `0004_billing.sql`, and
pin the SDK with `go get github.com/stripe/stripe-go/v79@latest`. For local webhook testing
use the Stripe CLI: `stripe listen --forward-to localhost:8080/api/webhooks/stripe`.

## Accounts & team

Registration creates an organization and its first admin. Admins can **invite teammates**
(staff or admin) from the Team page — invites are tokenized links, emailed when SMTP is
configured and always shown in-app so the link can be shared directly. Invitees set their
own password via the link. **Password reset** works the same way: request a link from the
sign-in page; the API never reveals whether an email is registered, and reset tokens are
single-use and expire in an hour. Alert, invite, and reset emails all go through one
`internal/email` mailer (logs to console when no SMTP server is set).

## Roadmap

- Native mobile.

## Note on temperature thresholds

The default safe ranges (fridge 33–40, freezer −10–10, hot-hold 135–165 °F) are sensible
starting points, **not** legal guidance — thresholds vary by state and local health
department. Let each operator set their own ranges per unit.
