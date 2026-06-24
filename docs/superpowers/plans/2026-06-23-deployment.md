# ChillCheck Production Deployment — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Produce an educational `DEPLOYMENT.md` runbook that takes ChillCheck from "exists only locally" to a live AWS pilot (App Runner API + RDS + Amplify frontend + SES email + Stripe billing + on-site BLE gateway), and bump the Go toolchain to the latest version.

**Architecture:** The deliverable is mostly documentation — a single `DEPLOYMENT.md` written in dependency order, each step shaped as **What you run · Why this choice · Gotchas**. One real code change accompanies it: bump both Go modules and the API Dockerfile to Go 1.26, gated on a green `go test`. No infrastructure-as-code; the runbook is the future Terraform spec.

**Tech Stack:** AWS (App Runner, ECR, RDS PostgreSQL, Amplify Hosting, Route 53, ACM, SES, Secrets Manager, VPC connector), Stripe, Go 1.26, Docker, a Raspberry Pi (arm64) for the gateway.

## Global Constraints

- **Domain (example):** `chillcheck.app`. **Region:** `us-east-1`. **Gateway hardware:** Raspberry Pi, `GOARCH=arm64`. Use these consistently in every command and placeholder.
- **Never put real secrets in tracked files.** `.env` is gitignored; production secrets live only in AWS Secrets Manager. The runbook must never instruct pasting a real secret into a tracked file or a git commit.
- **API env vars (exact names, from `backend/internal/config`):** `DATABASE_URL`, `JWT_SECRET`, `PORT`, `CORS_ORIGIN`, `ALERTS_ENABLED`, `ALERT_INTERVAL`, `SMTP_HOST`, `SMTP_PORT`, `SMTP_USER`, `SMTP_PASS`, `ALERT_FROM`, `STRIPE_SECRET_KEY`, `STRIPE_WEBHOOK_SECRET`, `STRIPE_PRICE_ID`, `APP_BASE_URL`. **Frontend:** `VITE_API_URL` (build-time; feeds the CSP `connect-src`). **Gateway:** YAML keys `api_url`, `gateway_key`, `sample_interval`, `spool_path`, `spool_max`; env overrides `CHILLCHECK_API_URL`, `CHILLCHECK_GATEWAY_KEY`.
- **Build order is mandatory:** domain → ACM/DNS → VPC/subnets/SG/connector → RDS + schema → Secrets Manager → ECR/image → App Runner → SES → Stripe → Amplify (needs final API URL) → gateway. SES sandbox-exit is *requested* in the prerequisites step (lead time) but *configured* later.
- **Doc style:** sentence-case headings, plain active voice (matches repo copy conventions). Every step shows the actual command — no "TODO"/"configure appropriately" placeholders.
- **Spec:** `docs/superpowers/specs/2026-06-23-deployment-design.md` is the source of truth for decisions; this plan implements it.

---

### Task 1: Bump Go toolchain to 1.26 and verify the build

**Files:**
- Modify: `backend/go.mod:3` (`go 1.23` → `go 1.26.4`)
- Modify: `gateway/go.mod:3` (`go 1.23` → `go 1.26.4`)
- Modify: `backend/Dockerfile:2` (`FROM golang:1.23 AS build` → `FROM golang:1.26 AS build`)

**Interfaces:**
- Consumes: nothing.
- Produces: a green backend test suite on Go 1.26; an API image that builds on `golang:1.26`. No exported symbols change.

- [ ] **Step 1: Confirm the current toolchain is 1.26.x**

Run: `go version`
Expected: `go version go1.26.4 linux/amd64` (or newer 1.26 patch). If it prints something older, stop — install Go 1.26 first; the bump must not outrun the installed toolchain.

- [ ] **Step 2: Establish the green baseline before changing anything**

Run: `make test`
Expected: `ok` for `chillcheck/internal/store` and `chillcheck/internal/api` (other packages `no test files`). This proves any later failure came from the bump.

- [ ] **Step 3: Bump `backend/go.mod`**

Change line 3 from `go 1.23` to:

```
go 1.26.4
```

- [ ] **Step 4: Bump `gateway/go.mod`**

Change line 3 from `go 1.23` to:

```
go 1.26.4
```

- [ ] **Step 5: Bump the Dockerfile base image**

In `backend/Dockerfile`, change line 2 from `FROM golang:1.23 AS build` to:

```dockerfile
FROM golang:1.26 AS build
```

- [ ] **Step 6: Tidy and re-run the backend tests**

Run: `cd backend && go mod tidy && go test ./... && cd ..`
Expected: same `ok` lines as Step 2, no errors. If `go mod tidy` reports a dependency needing a newer Go than 1.26, stop and report — do not force it.

- [ ] **Step 7: Verify the gateway still builds**

Run: `cd gateway && go build ./... && cd ..`
Expected: no output (success).

- [ ] **Step 8: Verify the production image builds on the new base**

Run: `docker build -t chillcheck-api:go126-check backend`
Expected: build succeeds through both stages; final line `naming to docker.io/library/chillcheck-api:go126-check`. (If Docker isn't available in this environment, note it and rely on Step 6/7; the real build happens in Task 5.)

- [ ] **Step 9: Commit**

```bash
git add backend/go.mod backend/go.sum gateway/go.mod backend/Dockerfile
git commit -m "Bump Go toolchain to 1.26"
```

(Include `gateway/go.sum` in the `add` if `go mod tidy` changed it.)

---

### Task 2: Scaffold DEPLOYMENT.md — overview, checklist, prerequisites (§0–2)

**Files:**
- Create: `DEPLOYMENT.md` (repo root)
- Modify: `README.md` (add a link to the runbook)

**Interfaces:**
- Consumes: the spec's decision table.
- Produces: the document skeleton with all 16 section headings (§0–15) present as `##` headers so later tasks fill them in order; §1 checklist and §2 prerequisites complete.

- [ ] **Step 1: Create `DEPLOYMENT.md` with the full heading skeleton and §0–2 written**

Create `DEPLOYMENT.md`. Start with this intro and the section headers; write §0, §1, §2 in full now, leave §3–15 as empty `##` headers (later tasks fill them):

```markdown
# Deploying ChillCheck to AWS

A step-by-step, explain-the-why runbook for the first production deploy. It assumes an
existing AWS account and **no domain yet**, region `us-east-1`, and a Raspberry Pi for the
on-site gateway. For the system shape see `ARCHITECTURE.md`; for design rationale see
`docs/superpowers/specs/2026-06-23-deployment-design.md`.

Each step is shaped: **What you run · Why this choice · Gotchas.** Record every id, URL, and
secret name in the §1 checklist as you go.

## 0. Overview

ChillCheck is three programs plus a database. In production:

| Piece | Lands on | Reached at |
|---|---|---|
| Go API | App Runner (image from ECR) | `https://api.chillcheck.app` |
| PostgreSQL | RDS (private subnets) | via VPC connector only |
| React app | Amplify Hosting | `https://app.chillcheck.app` |
| Email | SES (SMTP) | `SMTP_*` env on the API |
| Billing | Stripe | webhook → `…/api/webhooks/stripe` |
| Gateway | Raspberry Pi on-site | calls the API with a gateway key |

Why this shape: App Runner runs the existing distroless container with managed TLS and
autoscaling and no cluster to operate; Lambda is avoided because Postgres connection
exhaustion bites serverless. RDS stays private and is reached only through an App Runner VPC
connector. The frontend is static, so Amplify hosts it cheaply and rebuilds from GitHub.
```

Then the §1 checklist (a table the operator fills in), §2 prerequisites. Continue in Steps 2–3.

- [ ] **Step 2: Write §1 — the pre-flight checklist**

Add a checklist the operator fills in as they progress. It must enumerate, with a blank to record each: AWS region (`us-east-1`), domain (`chillcheck.app`), Route 53 hosted zone ID, ACM certificate ARN, VPC id, two private subnet ids, DB security group id, App Runner VPC connector ARN, RDS endpoint + master password (→ Secrets Manager, not written here), `JWT_SECRET` (→ Secrets Manager), ECR repo URI, App Runner service ARN + default domain, `api.chillcheck.app`, SES verified domain + SMTP username/password (→ Secrets Manager), Stripe live secret key / price id / webhook secret (→ Secrets Manager), Amplify app id + `app.chillcheck.app`, gateway key (→ created in §12). Mark secret rows explicitly "stored in Secrets Manager, not in this file."

- [ ] **Step 3: Write §2 — prerequisites**

Cover, each as What/Why/Gotchas:
- **AWS CLI v2 installed and configured** — `aws --version`; `aws configure` (or SSO). Set default region: `aws configure set region us-east-1`.
- **Docker installed** — `docker --version`; needed to build/push the API image.
- **IAM admin user/role** — don't use the account root for daily work; create an admin IAM user with MFA. Why: blast-radius and a recoverable root.
- **Billing alarm** — create a CloudWatch billing alarm (e.g. $50) so a misconfigured resource can't run up a surprise. Why: pilots leak money via forgotten NAT gateways / oversized RDS.
- **Request SES production access NOW** — Console → SES → Account dashboard → "Request production access". Why: by default SES is sandboxed (can only send to verified addresses); approval has lead time, so request it first even though you configure SES in §9.

- [ ] **Step 4: Add §3–15 as empty headers**

Append the remaining headers exactly so later tasks fill them in place:

```markdown
## 3. Domain (Route 53)
## 4. Networking (VPC, subnets, security groups, VPC connector)
## 5. RDS PostgreSQL + one-time schema load
## 6. Secrets Manager
## 7. ECR + build and push the API image
## 8. App Runner
## 9. SES (email)
## 10. Stripe (billing)
## 11. Frontend on Amplify
## 12. Gateway on a Raspberry Pi
## 13. Smoke test
## 14. Cost estimate and teardown
## 15. Codify in Terraform later
```

- [ ] **Step 5: Link the runbook from `README.md`**

Add a line near the top of `README.md` (e.g. under the run instructions): `**Deploying to AWS?** See [DEPLOYMENT.md](DEPLOYMENT.md).`

- [ ] **Step 6: Verify no placeholder leakage and headers are present**

Run: `grep -nE 'TODO|TBD|FIXME|configure appropriately|<fill' DEPLOYMENT.md; grep -c '^## ' DEPLOYMENT.md`
Expected: first grep prints nothing; second prints `16` (§0–15).

- [ ] **Step 7: Commit**

```bash
git add DEPLOYMENT.md README.md
git commit -m "Add deployment runbook scaffold (overview, checklist, prerequisites)"
```

---

### Task 3: Infra core — domain, networking, RDS (§3–5)

**Files:**
- Modify: `DEPLOYMENT.md` (fill §3, §4, §5)

**Interfaces:**
- Consumes: §1 checklist fields.
- Produces: prose + commands so the operator ends with a registered domain + hosted zone, a VPC with two private subnets, a DB security group, a VPC connector, and an RDS instance with `schema.sql` loaded and public access revoked.

- [ ] **Step 1: Write §3 — domain (Route 53)**

What/Why/Gotchas covering: register `chillcheck.app` via Route 53 (Console → Route 53 → Registered domains → Register), which auto-creates a hosted zone. Why Route 53: DNS, ACM validation, SES verification, and Amplify/App Runner custom domains all become records in one account. Record the hosted zone id in §1. Gotcha: registration can take minutes-to-hours to propagate; `.app` is on the HSTS preload list so it's HTTPS-only — fine here since everything is HTTPS.

- [ ] **Step 2: Write §4 — networking**

Cover, each What/Why/Gotchas:
- Use the **default VPC** (or a simple new one) and note **two private subnets** in different AZs (RDS needs a subnet group spanning ≥2 AZs). Record subnet ids.
- Create a **security group for RDS** allowing inbound `5432` only from the App Runner VPC connector's security group (and, temporarily, your own IP for the schema load in §5). Record the SG id.
- Create an **App Runner VPC connector** (Console → App Runner → VPC connectors, or `aws apprunner create-vpc-connector`) bound to those private subnets + a security group. Why: this is what lets App Runner reach a private RDS without giving the DB a public IP. Record the connector ARN. Gotcha: the connector's SG must be allowed inbound on the RDS SG — they're two different SGs.

- [ ] **Step 3: Write §5 — RDS + schema load**

Cover:
- Create the DB subnet group across the two private subnets.
- Create RDS PostgreSQL `db.t4g.micro`, single-AZ to start, storage encrypted, **not publicly accessible** in steady state. Set a strong master password — it goes to Secrets Manager in §6, not into this file. Record the endpoint.
- **One-time schema load:** temporarily set the instance to publicly accessible AND add your IP to the RDS SG, then:

```bash
psql "postgres://chillcheck:<master-pass>@<rds-endpoint>:5432/chillcheck?sslmode=require" -f backend/db/schema.sql
```

  Then **immediately** revoke public access and remove your IP from the SG. Why temporary public access: App Runner can't run psql for you and a bastion is more setup than a pilot needs; the window must be closed right after. Gotcha: production uses `sslmode=require` (local dev uses `disable`); the DB name/user must match `DATABASE_URL`.

- [ ] **Step 4: Verify §3–5 commands are concrete**

Run: `grep -nE 'TODO|TBD|configure appropriately' DEPLOYMENT.md` → expect nothing. Visually confirm every command uses `chillcheck.app` / `us-east-1` / real flag names.

- [ ] **Step 5: Commit**

```bash
git add DEPLOYMENT.md
git commit -m "Document domain, networking, and RDS steps"
```

---

### Task 4: API deploy — Secrets Manager, ECR, App Runner (§6–8)

**Files:**
- Modify: `DEPLOYMENT.md` (fill §6, §7, §8)

**Interfaces:**
- Consumes: RDS endpoint + master password (§5), VPC connector ARN (§4), ACM cert (§3).
- Produces: an App Runner service running the API at `https://api.chillcheck.app`, reading config from env + Secrets Manager.

- [ ] **Step 1: Write §6 — Secrets Manager**

Cover: generate `JWT_SECRET` with `openssl rand -base64 48`; store it plus the DB master password, and (placeholders for) Stripe + SMTP secrets, as Secrets Manager secrets:

```bash
aws secretsmanager create-secret --name chillcheck/jwt-secret --secret-string "$(openssl rand -base64 48)"
aws secretsmanager create-secret --name chillcheck/db-url --secret-string "postgres://chillcheck:<master-pass>@<rds-endpoint>:5432/chillcheck?sslmode=require"
```

Note that Stripe/SMTP secrets are added in §9/§10 when their values exist. Why Secrets Manager: App Runner injects these by reference so plaintext never lives in the service config or git. Gotcha: App Runner's instance role needs `secretsmanager:GetSecretValue` on these ARNs.

- [ ] **Step 2: Write §7 — ECR + build/push**

Cover, with exact commands:

```bash
aws ecr create-repository --repository-name chillcheck-api
aws ecr get-login-password --region us-east-1 | docker login --username AWS --password-stdin <acct>.dkr.ecr.us-east-1.amazonaws.com
docker build -t chillcheck-api backend
docker tag chillcheck-api:latest <acct>.dkr.ecr.us-east-1.amazonaws.com/chillcheck-api:latest
docker push <acct>.dkr.ecr.us-east-1.amazonaws.com/chillcheck-api:latest
```

Why ECR: App Runner deploys a prebuilt image (the existing distroless Dockerfile) rather than building from source. Gotcha: the image is `linux/amd64` for App Runner; if building on an Apple-silicon Mac add `--platform linux/amd64` to `docker build`.

- [ ] **Step 3: Write §8 — App Runner**

Cover:
- Create the service from the ECR image (Console → App Runner → Create service → Container registry). Port `8080`.
- **Non-secret env vars:** `PORT=8080`, `CORS_ORIGIN=https://app.chillcheck.app`, `APP_BASE_URL=https://app.chillcheck.app`, `ALERTS_ENABLED=true`, `ALERT_INTERVAL=1m`, `ALERT_FROM=alerts@chillcheck.app`, `SMTP_PORT=587`.
- **Secret env vars (Secrets Manager references):** `DATABASE_URL` → `chillcheck/db-url`, `JWT_SECRET` → `chillcheck/jwt-secret` (Stripe/SMTP added later).
- Attach the **VPC connector** from §4 so it can reach RDS.
- Add the **custom domain** `api.chillcheck.app` (App Runner → Custom domains) and create the CNAME/validation records it gives you in Route 53; it provisions its own managed cert. Record the service ARN and default domain.

Gotcha: a failed health check usually means `DATABASE_URL` is wrong or the VPC connector SG isn't allowed into the RDS SG — check both. App Runner manages TLS, so the API has no cert config of its own.

- [ ] **Step 4: Verify and commit**

Run: `grep -nE 'TODO|TBD|configure appropriately' DEPLOYMENT.md` → expect nothing.

```bash
git add DEPLOYMENT.md
git commit -m "Document Secrets Manager, ECR, and App Runner steps"
```

---

### Task 5: Email and billing — SES, Stripe (§9–10)

**Files:**
- Modify: `DEPLOYMENT.md` (fill §9, §10)

**Interfaces:**
- Consumes: the SES production-access request from §2; the App Runner service from §8; `api.chillcheck.app`.
- Produces: real outbound email via SES SMTP wired into the API; a live Stripe webhook syncing subscription state.

- [ ] **Step 1: Write §9 — SES**

Cover:
- Verify the **domain** `chillcheck.app` in SES (Console → SES → Identities) and add the **DKIM** CNAME records it provides to Route 53.
- Confirm **production access** (requested in §2) is granted; until then SES only delivers to verified addresses.
- Create **SMTP credentials** (SES → SMTP settings → Create SMTP credentials — this makes an IAM user with SMTP-specific username/password). Store them: `aws secretsmanager create-secret --name chillcheck/smtp --secret-string '{"user":"...","pass":"..."}'` (or two secrets).
- Add to App Runner: `SMTP_HOST=email-smtp.us-east-1.amazonaws.com`, `SMTP_USER`/`SMTP_PASS` from the secret. The app's `SMTPMailer` activates as soon as `SMTP_HOST` is set (STARTTLS:587), and the alert engine + invites + password resets all start sending real mail.

Gotcha: SES SMTP username/password are **not** your AWS access keys — they're generated specifically by the SMTP-credentials flow. `ALERT_FROM` must be on the verified domain.

- [ ] **Step 2: Write §10 — Stripe**

Cover:
- In the Stripe dashboard (live mode): create a **product + recurring price** (per-location). Record the price id (`STRIPE_PRICE_ID`).
- Get the **live secret key** (`STRIPE_SECRET_KEY`).
- Create a **webhook endpoint** → `https://api.chillcheck.app/api/webhooks/stripe`, subscribe to the subscription/customer events; record the **signing secret** (`STRIPE_WEBHOOK_SECRET`).
- Store all three in Secrets Manager and add them as App Runner env (referencing the secrets). Setting `STRIPE_SECRET_KEY` + `STRIPE_PRICE_ID` is what flips `BillingEnabled()` on; until then billing is fully disabled.

Gotcha: subscription state syncs **only** via the verified webhook — a wrong `STRIPE_WEBHOOK_SECRET` silently breaks sync with no client-visible error. `APP_BASE_URL` (set in §8) is where Checkout/Portal redirect back, so it must be the frontend origin.

- [ ] **Step 3: Verify and commit**

Run: `grep -nE 'TODO|TBD|configure appropriately' DEPLOYMENT.md` → expect nothing.

```bash
git add DEPLOYMENT.md
git commit -m "Document SES email and Stripe billing steps"
```

---

### Task 6: Frontend and gateway — Amplify, Raspberry Pi (§11–12)

**Files:**
- Modify: `DEPLOYMENT.md` (fill §11, §12)

**Interfaces:**
- Consumes: the final API URL `https://api.chillcheck.app` (§8); the gateway-create UI route `POST /api/locations/{id}/gateways`.
- Produces: the React app served at `https://app.chillcheck.app`; a gateway agent running on a Pi, store-and-forwarding readings to the API.

- [ ] **Step 1: Write §11 — Amplify**

Cover:
- Connect the GitHub repo in Amplify Hosting; set the app root to `frontend/`, build = `npm run build`, output = `dist/`.
- Set the **build-time env var** `VITE_API_URL=https://api.chillcheck.app`. Why it must be set before the build: `frontend/vite.config.ts` derives the CSP `connect-src` from `VITE_API_URL`; if it's wrong the browser blocks API calls. If the API domain ever changes, **rebuild** the frontend.
- Add the custom domain `app.chillcheck.app` (Amplify → Domain management); it adds the Route 53 records and a managed cert. Record the app id.

Gotcha: a blank dashboard with CSP `connect-src` violations in the browser console = `VITE_API_URL` was wrong at build time; fix the var and redeploy.

- [ ] **Step 2: Write §12 — gateway on a Raspberry Pi**

Cover:
- **Create a gateway + key:** in the deployed app, open a location's gateway setup and create a gateway (`POST /api/locations/{id}/gateways`). The plaintext key (`chk_gw_…`) is shown **once** — record it for the Pi. Bind each sensor to a unit by MAC (`units.sensor_mac`, uppercase colon form).
- **Cross-compile for the Pi (arm64):**

```bash
cd gateway && CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o chillcheck-gateway . && cd ..
```

  Why standard Go (not TinyGo): `tinygo.org/x/bluetooth` is a normal Go module using BlueZ over D-Bus; the agent needs a real OS.
- **Install on the Pi:** copy the binary to `/opt/chillcheck-gateway/`, write `config.yaml` with `api_url: https://api.chillcheck.app` and `gateway_key: chk_gw_…` (or keep the key out of the file via `CHILLCHECK_GATEWAY_KEY` in the unit), install `gateway/systemd/chillcheck-gateway.service`, `systemctl enable --now chillcheck-gateway`.
- Why the spool matters: the agent store-and-forwards to disk on outages, so the compliance record stays gap-free; sensor readings keep their real measurement timestamp.

Gotcha: BLE scanning needs `CAP_NET_ADMIN`/`CAP_NET_RAW` (the unit sets them) or root; the `User=chillcheck` in the unit must exist on the Pi.

- [ ] **Step 3: Verify and commit**

Run: `grep -nE 'TODO|TBD|configure appropriately' DEPLOYMENT.md` → expect nothing.

```bash
git add DEPLOYMENT.md
git commit -m "Document Amplify frontend and Pi gateway steps"
```

---

### Task 7: Smoke test, cost/teardown, Terraform note (§13–15)

**Files:**
- Modify: `DEPLOYMENT.md` (fill §13, §14, §15)

**Interfaces:**
- Consumes: the full deployed stack.
- Produces: an end-to-end verification checklist, a cost/teardown reference, and a forward-looking IaC note. This task completes the runbook.

- [ ] **Step 1: Write §13 — smoke test**

An ordered end-to-end check against the live stack: register an org/admin at `https://app.chillcheck.app` → add a location + unit → log a manual temp → open the compliance PDF (confirm the integrity VERIFIED line) → force an out-of-range reading and confirm an alert email arrives via SES → `GET https://api.chillcheck.app/api/integrity` returns OK → with the Pi running, confirm a `source='sensor'` reading lands on the board. Each line states what proves the piece works (e.g. the alert email proves SES + the in-process alert engine + admin recipient lookup).

- [ ] **Step 2: Write §14 — cost estimate and teardown**

A rough monthly pilot estimate (App Runner smallest instance, `db.t4g.micro` RDS, Amplify, Route 53 hosted zone + domain, SES near-free at pilot volume) framed as "order of magnitude, check current pricing." Then teardown order (reverse of build) so nothing is orphaned: delete App Runner service, VPC connector, RDS (final snapshot decision), Amplify app, ECR repo, Secrets Manager secrets, SES identity, Stripe webhook; keep or release the domain. Why document teardown: forgotten resources (especially anything with an idle hourly charge) are the main way a pilot leaks money.

- [ ] **Step 3: Write §15 — codify in Terraform later**

A short note: this runbook is the spec for a future Terraform module; once the manual setup is stable, codify it for repeatable/multi-environment provisioning and a GitHub Actions build→ECR→App Runner pipeline. Not now — YAGNI for a single pilot.

- [ ] **Step 4: Final whole-document review**

Run: `grep -nE 'TODO|TBD|FIXME|configure appropriately|<fill' DEPLOYMENT.md; grep -c '^## ' DEPLOYMENT.md`
Expected: first prints nothing; second prints `16`. Read the doc top-to-bottom once: confirm build-order dependencies are satisfied (no step needs a value produced by a later step except the deliberately-early SES request), and `chillcheck.app`/`us-east-1`/arm64 are consistent throughout.

- [ ] **Step 5: Commit**

```bash
git add DEPLOYMENT.md
git commit -m "Document smoke test, cost/teardown, and Terraform note"
```

---

## Self-review

**Spec coverage:** §3–15 of the runbook map 1:1 to the spec's `DEPLOYMENT.md` structure (Tasks 2–7); the Go 1.26 bump (spec "Go toolchain bump") is Task 1; the README link and Dockerfile/`go.mod` changes (spec "Repo changes bundled") are in Tasks 1–2. Every spec "Risks/things to watch" item has a home: SES sandbox lead time (§2 + §9), VITE_API_URL ordering (§11), schema-load access window (§5), Stripe webhook secret (§10), secrets-never-in-git (Global Constraints + §6).

**Placeholder scan:** `<acct>`, `<rds-endpoint>`, `<master-pass>` are operator-supplied runtime values, not plan placeholders — they're explicitly described as "record this id." No `TODO`/`TBD`/"configure appropriately" in actionable steps; each doc task ends with a grep that enforces their absence.

**Type/name consistency:** env var names, gateway YAML keys, and routes are copied verbatim from the codebase (Global Constraints) and reused identically across §6/§8/§9/§10/§11/§12. Section count (16) is asserted in Tasks 2 and 7.
