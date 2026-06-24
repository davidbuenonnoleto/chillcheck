# ChillCheck — Production Deployment Design

Date: 2026-06-23
Status: Approved (design); runbook to follow

This document captures the architecture decisions and structure for ChillCheck's first
production deployment to AWS. The deliverable it drives is an educational `DEPLOYMENT.md`
runbook — a step-by-step guide that explains *what* to run, *why* each choice was made, and
*what can go wrong*. See `ARCHITECTURE.md` for the system topology this builds on.

## Goal

Get a usable, secure pilot live end-to-end: API, database, frontend, real email, live
billing, and an on-site BLE gateway — and teach the operator how the pieces fit together
while doing it. This is the first deploy; nothing is in AWS yet (the AWS account exists, no
domain is registered).

## Non-goals

- Infrastructure-as-code (Terraform/CDK) — deferred. The manual runbook is the spec a future
  Terraform module will codify, once the setup is stable. Doing it by hand once is the
  fastest way to *learn* the moving parts.
- CI/CD pipeline (GitHub Actions build/deploy) — deferred to the same "later" note.
- Multi-environment (staging/prod split), blue-green, autoscaling tuning, observability
  stack. Pilot scale only.

## Why a runbook, not IaC (for now)

- You learn the system by touching each resource once. Terraform hides the moving parts
  behind `terraform apply`; that intuition is what makes IaC readable later.
- IaC pays off on the 2nd/3rd environment or with a team needing repeatable provisioning —
  neither exists yet. Writing it now is maintenance burden against a setup still in flux.
- The runbook becomes the authoritative spec for the eventual Terraform; it's already proven
  correct because it's running.

## Target architecture & key decisions

Matches `ARCHITECTURE.md`, with production specifics made explicit.

| Concern | Decision | Rationale |
|---|---|---|
| API hosting | **App Runner**, container pulled **from ECR** | `backend/Dockerfile` already builds a distroless static binary. App Runner *source* builds don't take arbitrary Dockerfiles cleanly, so the standard path is `docker build` → push **ECR** → App Runner deploys it. Managed TLS, autoscaling, no cluster. Lambda is ruled out (Postgres connection exhaustion — see CLAUDE.md). |
| API → DB connectivity | **VPC connector** + RDS in **private subnets** | DB has no public IP; only App Runner (via the connector) reaches it. The public-RDS + IP-allowlist alternative is documented as a tradeoff but not recommended (App Runner egress IPs aren't static without the connector). |
| Database | **RDS PostgreSQL**, single `db.t4g.micro` to start | Aurora is overkill/cost for a pilot; easy to scale up later. Schema loaded **once** via `psql` using temporary public access from the operator's IP, then locked down. |
| Secrets | **AWS Secrets Manager** for `JWT_SECRET`, DB password, Stripe keys, `SMTP_PASS`; plain env vars for non-secret config | App Runner references Secrets Manager natively; teaches the right habit vs. pasting secrets into the console. |
| Frontend | **Amplify Hosting**, builds from GitHub, `VITE_API_URL` → API domain | Documented target. The build-time CSP `connect-src` derives from `VITE_API_URL`, so the **API domain must be final before the frontend build**. |
| Domain / DNS | **Register in Route 53** (e.g. `chillcheck.app`) | One account for DNS + ACM validation + SES verification + Amplify/App Runner custom domains — everything is a DNS record in one place. `api.<domain>` → App Runner; apex or `app.<domain>` → Amplify. |
| Email | **SES**: verify domain + DKIM, **exit sandbox**, SMTP creds → app's existing `SMTPMailer` (STARTTLS:587) | Sandbox exit is a support request with lead time — flagged at the **start** of the runbook so it's approved by the time it's needed. |
| Billing | **Stripe** live keys, product/price, webhook → `https://api.<domain>/api/webhooks/stripe` | Webhook secret + `APP_BASE_URL` = frontend origin. Subscription state syncs only via the verified webhook. |
| Gateway | Cross-compile **arm64**, install via existing `gateway/systemd` unit on a Pi, pointed at the API URL + a gateway key | Already designed for this: `CGO_ENABLED=0 GOOS=linux GOARCH=arm64`. Store-and-forward spool keeps the compliance record gap-free across outages. |

### Go toolchain bump

The operator wants the latest Go. Installed toolchain is **Go 1.26.4**; both `go.mod` files
declare `go 1.23`. As part of this work:

- `backend/go.mod` and `gateway/go.mod` → `go 1.26.4`
- `backend/Dockerfile` base → `golang:1.26`
- Verify `go build ./...` / `go test ./...` still pass after the bump before relying on the
  image. (The bump is a real code change, gated on a green build — not assumed.)

## Build order (the runbook enforces this)

Dependencies between resources dictate a strict order:

```
domain (Route 53)
  → ACM cert + DNS validation
  → VPC / subnets / security groups / VPC connector
  → RDS Postgres + one-time schema load
  → Secrets Manager (JWT, DB, Stripe, SMTP)
  → ECR + build/push API image
  → App Runner (env + secrets + VPC connector + custom domain api.<domain>)
  → SES (domain verify, DKIM, sandbox exit, SMTP creds)
  → Stripe (product/price, webhook, secrets)
  → Frontend on Amplify (needs final API URL for VITE_API_URL/CSP)
  → Gateway on Pi (needs API URL + gateway key)
```

SES sandbox-exit is *requested* early (lead time) even though it's *configured* mid-runbook.

## `DEPLOYMENT.md` structure

Each step uses a consistent shape: **What you run** (CLI/console) · **Why this choice** ·
**Gotchas**. Every secret/ID gets a placeholder the operator records in the §1 checklist as
they go.

```
0.  Overview + architecture recap (condensed decision table)
1.  ✅ Pre-flight CHECKLIST — every resource, secret, DNS record at a glance
2.  Prerequisites — AWS CLI, Docker, IAM admin user, billing alarm, SES sandbox-exit request
3.  Domain — Route 53 register + hosted zone
4.  Networking — VPC, private subnets, security groups, VPC connector
5.  RDS Postgres + one-time schema load
6.  Secrets Manager — generate JWT secret; store DB/Stripe/SMTP
7.  ECR + build/push the API image (with the Go 1.26 bump)
8.  App Runner — env, secrets, VPC connector, custom domain api.<domain>
9.  SES — domain verify, DKIM, sandbox exit, SMTP creds
10. Stripe — product/price, webhook endpoint, secrets
11. Frontend on Amplify — VITE_API_URL, custom domain, CSP note
12. Gateway on a Pi — cross-compile, gateway key, systemd install
13. Smoke test — register → log temp → PDF → alert email → sensor ingest
14. Cost estimate + teardown
15. "Codify in Terraform later" note
```

## Repo changes bundled with this work

- New `DEPLOYMENT.md` at repo root.
- `README.md` — link to `DEPLOYMENT.md`.
- `backend/go.mod`, `gateway/go.mod` — `go 1.26.4`.
- `backend/Dockerfile` — `FROM golang:1.26`.
- Verify backend tests pass after the Go bump (`make test`).

## Risks / things to watch

- **SES sandbox exit lead time** — request first; alert/invite/reset email won't reach
  arbitrary recipients until approved.
- **VITE_API_URL ordering** — the frontend build bakes the API origin into the CSP; rebuild
  the frontend if the API domain changes.
- **Schema load access window** — RDS is briefly publicly reachable from the operator's IP to
  load `schema.sql`; the runbook must close that window (revoke the SG rule) immediately
  after.
- **Stripe webhook secret** — billing state syncs only via the verified webhook; a wrong
  secret silently breaks subscription sync.
- **Secrets never in git** — `.env` is gitignored; production secrets live only in Secrets
  Manager. The runbook must never instruct pasting real secrets into tracked files.
```
