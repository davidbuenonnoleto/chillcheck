# Deploying ChillCheck to AWS

A step-by-step, explain-the-why runbook for the first production deploy. It assumes an
existing AWS account and **no domain yet**, region `us-east-1`, and a Raspberry Pi for the
on-site gateway. For the system shape see `ARCHITECTURE.md`; for design rationale see
`docs/superpowers/specs/2026-06-23-deployment-design.md`.

Each step is shaped: **What you run · Why this choice · Gotchas.** Record every id, URL, and
secret name in the [§1 checklist](#1-pre-flight-checklist) as you go. Work top to bottom —
the order encodes dependencies (a later step needs a value an earlier one produced).

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
connector. The frontend is static, so Amplify hosts it cheaply and rebuilds from GitHub. The
gateway is the only on-site piece; it talks to the API like any other client.

A note on cost discipline: almost everything here is pay-per-use or near-free at pilot
volume, **except** anything billed by the hour whether or not you use it (a NAT gateway, an
oversized RDS, an idle App Runner). Set the billing alarm in §2 before you create anything.

## 1. Pre-flight checklist

Fill these in as you go. Rows marked **(secret)** are stored in AWS Secrets Manager, **never
written into this file or committed to git**.

| Item | Value | Where it's set |
|---|---|---|
| Region | `us-east-1` | everywhere |
| Domain | `chillcheck.app` | §3 |
| Route 53 hosted zone id | `__________` | §3 |
| ACM / managed cert | (App Runner & Amplify manage their own) | §8, §11 |
| VPC id | `__________` | §4 |
| Private subnet A id | `__________` | §4 |
| Private subnet B id | `__________` | §4 |
| RDS security group id | `__________` | §4 |
| VPC connector ARN | `__________` | §4 |
| RDS endpoint | `__________` | §5 |
| RDS master password | **(secret)** → `chillcheck/db-url` | §5/§6 |
| `JWT_SECRET` | **(secret)** → `chillcheck/jwt-secret` | §6 |
| ECR repo URI | `__________` | §7 |
| App Runner service ARN | `__________` | §8 |
| App Runner default domain | `__________` | §8 |
| API custom domain | `api.chillcheck.app` | §8 |
| SES verified domain | `chillcheck.app` | §9 |
| SES SMTP username/password | **(secret)** → `chillcheck/smtp` | §9 |
| Stripe secret key | **(secret)** → `chillcheck/stripe` | §10 |
| Stripe price id | `__________` (not secret, but keep with the rest) | §10 |
| Stripe webhook secret | **(secret)** → `chillcheck/stripe` | §10 |
| Amplify app id | `__________` | §11 |
| Frontend custom domain | `app.chillcheck.app` | §11 |
| Gateway key | **(secret, shown once)** | §12 |

## 2. Prerequisites

**AWS CLI v2 — installed and configured.**
What: `aws --version` (expect v2.x). Configure credentials with `aws configure` (or SSO), then
pin the region: `aws configure set region us-east-1`.
Why: most steps below are CLI; pinning the region avoids resources landing in the wrong one.
Gotcha: if `aws sts get-caller-identity` shows the account root, stop and switch to an admin
user (next item).

**An IAM admin user with MFA — not the account root.**
What: in IAM, create a user with `AdministratorAccess` and MFA; use it for everything here.
Why: the root account should be locked away with MFA and used only for the handful of things
that require it. Day-to-day work on the root is an unrecoverable blast radius.
Gotcha: rotate/disable any root access keys.

**Docker — installed.**
What: `docker --version`. Needed to build and push the API image in §7.
Why: App Runner deploys a prebuilt image; you build it locally and push to ECR.

**A billing alarm.**
What: CloudWatch → Alarms → create a billing alarm (e.g. notify at $50/month) on the
`EstimatedCharges` metric (which lives in `us-east-1`).
Why: a pilot's biggest financial risk is a forgotten hourly-billed resource. The alarm is your
tripwire.

**Request SES production access — now.**
What: Console → SES (in `us-east-1`) → Account dashboard → **Request production access**, with
a short description of the use (transactional compliance alerts, team invites, password
resets).
Why: by default SES is **sandboxed** — it only delivers to addresses you've verified, which
would silently break real alert/invite emails. Approval takes time (often a day), so request
it first even though you finish SES config in §9.
Gotcha: nothing else here blocks on it, so kick it off and keep going.

## 3. Domain (Route 53)

**What you run.** Console → Route 53 → **Registered domains** → **Register domains** → search
for `chillcheck.app`, complete registration. Registration automatically creates a **hosted
zone** for the domain. Record the hosted zone id in §1.

**Why this choice.** Registering in Route 53 keeps DNS, ACM certificate validation, SES domain
verification, and the Amplify/App Runner custom domains all in one account — every one of
those is "add a record to the hosted zone," with no third-party DNS to coordinate.

**Gotchas.** Registration can take minutes to a few hours to finalize; you can proceed with §4
meanwhile. `.app` is on the browser **HSTS preload** list, so it's HTTPS-only by design —
that's fine here since every endpoint is HTTPS, but it means there is no plain-HTTP fallback to
test with.

## 4. Networking (VPC, subnets, security groups, VPC connector)

The goal of this section: give App Runner a private path to RDS so the database never has a
public IP in steady state.

**Pick a VPC and two private subnets.**
What: use the account's **default VPC** (simplest) and note **two subnets in different
Availability Zones** to use as "private" for the DB. Record both subnet ids.
Why two: RDS subnet groups require subnets in at least two AZs. Two AZs also let you enable
Multi-AZ later without re-architecting.

**Create the RDS security group.**
What: create a security group (e.g. `chillcheck-rds-sg`) in that VPC. Inbound rule: TCP
`5432` from the **VPC connector's** security group (created next). You'll also add a temporary
rule for your own IP during the one-time schema load in §5, then remove it. Record the SG id.
Why: the DB accepts connections only from the App Runner connector — not from the internet.

**Create the App Runner VPC connector.**
What: Console → App Runner → **VPC connectors** → create one bound to the two subnets and its
own security group (e.g. `chillcheck-conn-sg`). Or:

```bash
aws apprunner create-vpc-connector \
  --vpc-connector-name chillcheck-conn \
  --subnets <subnet-a> <subnet-b> \
  --security-groups <chillcheck-conn-sg-id>
```

Record the connector ARN.
Why: this connector is what lets the App Runner service reach a private RDS. Without it, App
Runner has only public egress (with non-static IPs), which is why a public DB + IP allowlist is
the wrong pattern here.

**Gotcha.** The connector SG and the RDS SG are **two different** security groups. The RDS SG's
inbound rule must reference the **connector's** SG as its source. Getting these crossed is the
most common reason the API can't reach the database later.

## 5. RDS PostgreSQL + one-time schema load

**Create the DB subnet group.**
What: RDS → Subnet groups → create one spanning the two subnets from §4.

**Create the instance.**
What: RDS → Create database → PostgreSQL, **`db.t4g.micro`**, single-AZ, **storage
encrypted**, **Public access: No**, attach the `chillcheck-rds-sg`, and the subnet group above.
Set DB name `chillcheck`, master user `chillcheck`, and a strong master password. Record the
endpoint; the password goes to Secrets Manager in §6, **not** into this file.
Why `db.t4g.micro` single-AZ: right-sized and cheap for a pilot; scale up or enable Multi-AZ
later with no app changes. Encryption-at-rest is free and expected for a compliance product.

**Load the schema (one time, then lock down).**
What: temporarily set the instance to **Publicly accessible: Yes** *and* add your current IP to
`chillcheck-rds-sg` (inbound `5432` from your IP). Then load the DDL:

```bash
psql "postgres://chillcheck:<master-pass>@<rds-endpoint>:5432/chillcheck?sslmode=require" \
  -f backend/db/schema.sql
```

Immediately afterward, set **Publicly accessible: No** again and **remove** your IP rule from
the SG.
Why the temporary window: App Runner can't run `psql` for you, and standing up a bastion host
is more than a pilot needs. Opening a narrow, briefly-lived path from your own IP is the
pragmatic move — as long as you close it right after.

**Gotchas.** Production uses `sslmode=require` (local dev uses `disable`). The DB name and user
in the connection string must match what the API's `DATABASE_URL` will use (§6). If you later
add migrations, run them through this same temporary-access pattern or a bastion — there's no
auto-migration on boot.

## 6. Secrets Manager

**What you run.** Generate the JWT secret and store it plus the database URL. Add the Stripe
and SMTP secrets in §9/§10 once those values exist.

```bash
aws secretsmanager create-secret --name chillcheck/jwt-secret \
  --secret-string "$(openssl rand -base64 48)"

aws secretsmanager create-secret --name chillcheck/db-url \
  --secret-string "postgres://chillcheck:<master-pass>@<rds-endpoint>:5432/chillcheck?sslmode=require"
```

**Why this choice.** App Runner can inject secret values **by reference**, so plaintext never
appears in the service configuration, the console UI, or git. A long random `JWT_SECRET` is
essential — the app warns loudly if the dev default is left in place.

**Gotchas.** The App Runner instance role needs `secretsmanager:GetSecretValue` on exactly
these secret ARNs (the create-service flow can attach this). Keep the secret **names** stable;
they're referenced from the App Runner config in §8.

## 7. ECR + build and push the API image

**What you run.**

```bash
# one-time: create the repository
aws ecr create-repository --repository-name chillcheck-api

# authenticate Docker to ECR (replace <acct> with your account id)
aws ecr get-login-password --region us-east-1 \
  | docker login --username AWS --password-stdin <acct>.dkr.ecr.us-east-1.amazonaws.com

# build, tag, push
docker build -t chillcheck-api backend
docker tag chillcheck-api:latest <acct>.dkr.ecr.us-east-1.amazonaws.com/chillcheck-api:latest
docker push <acct>.dkr.ecr.us-east-1.amazonaws.com/chillcheck-api:latest
```

Record the repo URI in §1.

**Why this choice.** App Runner deploys the prebuilt distroless image from `backend/Dockerfile`
rather than building from source, so what runs in production is exactly what you built and
tested locally. The image builds on `golang:1.26` (the toolchain this repo targets).

**Gotchas.** App Runner runs **`linux/amd64`**. If you build on an Apple-silicon Mac, add
`--platform linux/amd64` to `docker build`, or the service will fail to start with an exec
format error.

## 8. App Runner

**What you run.** Console → App Runner → **Create service** → **Container registry** → the ECR
image from §7. Port **`8080`**. Configure the environment:

**Non-secret env vars** (plain values):

| Var | Value |
|---|---|
| `PORT` | `8080` |
| `CORS_ORIGIN` | `https://app.chillcheck.app` |
| `APP_BASE_URL` | `https://app.chillcheck.app` |
| `ALERTS_ENABLED` | `true` |
| `ALERT_INTERVAL` | `1m` |
| `ALERT_FROM` | `alerts@chillcheck.app` |
| `SMTP_PORT` | `587` |

**Secret env vars** (Secrets Manager references):

| Var | Secret |
|---|---|
| `DATABASE_URL` | `chillcheck/db-url` |
| `JWT_SECRET` | `chillcheck/jwt-secret` |

(`SMTP_*` and `STRIPE_*` are added in §9 and §10, once those secrets exist.)

Attach the **VPC connector** from §4 so the service can reach RDS. Then add the **custom
domain** `api.chillcheck.app` (App Runner → Custom domains) and create the CNAME/validation
records it shows you in the Route 53 hosted zone; App Runner provisions and renews its own
managed TLS cert. Record the service ARN and the default `*.awsapprunner.com` domain.

**Why this choice.** App Runner gives a managed-TLS, autoscaling HTTPS service from a container
with no load balancer or cluster to run. The in-process alert engine runs inside this same
service (it's a goroutine in the API), so enabling `ALERTS_ENABLED` here is all the alerting
needs — and it's safe across multiple instances because each tick takes a Postgres advisory
lock.

**Gotchas.** If the service's health check fails on first deploy, it's almost always one of: a
wrong `DATABASE_URL`, or the **VPC connector SG not allowed inbound on the RDS SG** (revisit
§4). App Runner manages TLS, so the API itself has no certificate configuration. Setting
`CORS_ORIGIN` to the exact frontend origin matters — a mismatch shows up as browser CORS errors
once the frontend is live.

## 9. SES (email)

**Verify the domain.**
What: SES (in `us-east-1`) → **Identities** → create a **domain identity** for `chillcheck.app`,
and add the **DKIM** CNAME records it generates to the Route 53 hosted zone.
Why: a verified domain with DKIM lets the app send as `alerts@chillcheck.app` with good
deliverability.

**Confirm production access.**
What: check that the request from §2 is granted (SES → Account dashboard). Until it is, SES
only delivers to verified addresses.

**Create SMTP credentials.**
What: SES → **SMTP settings** → **Create SMTP credentials**. This creates an IAM user whose
SMTP username/password are **not** your AWS keys. Store them:

```bash
aws secretsmanager create-secret --name chillcheck/smtp \
  --secret-string '{"user":"<ses-smtp-user>","pass":"<ses-smtp-pass>"}'
```

**Wire into App Runner.**
What: add env to the App Runner service: `SMTP_HOST=email-smtp.us-east-1.amazonaws.com`, and
`SMTP_USER`/`SMTP_PASS` referenced from `chillcheck/smtp`.
Why: the API's mailer switches from log-only to real SMTP the moment `SMTP_HOST` is set — and
that one mailer is shared by the alert engine, team invites, and password resets, so all three
start sending real email at once.

**Gotchas.** SES SMTP credentials are generated specifically by the SMTP-settings flow; don't
try to use your IAM access keys. `ALERT_FROM` must be on the verified domain
(`alerts@chillcheck.app`), or SES rejects the send.

## 10. Stripe (billing)

**Create the product and price.**
What: Stripe dashboard (**live mode**) → Products → create a product with a **recurring price**
(this is the per-location price). Record the price id → `STRIPE_PRICE_ID`.

**Get the secret key.**
What: Developers → API keys → the **live secret key** → `STRIPE_SECRET_KEY`.

**Create the webhook endpoint.**
What: Developers → Webhooks → add endpoint → URL
`https://api.chillcheck.app/api/webhooks/stripe`, subscribe to the subscription/customer
lifecycle events. Record the **signing secret** → `STRIPE_WEBHOOK_SECRET`.

**Store and wire.**
What: store all three in Secrets Manager and reference them as App Runner env
(`STRIPE_SECRET_KEY`, `STRIPE_PRICE_ID`, `STRIPE_WEBHOOK_SECRET`):

```bash
aws secretsmanager create-secret --name chillcheck/stripe \
  --secret-string '{"secret_key":"sk_live_...","price_id":"price_...","webhook_secret":"whsec_..."}'
```

**Why this choice.** Setting `STRIPE_SECRET_KEY` + `STRIPE_PRICE_ID` is exactly what flips the
app's `BillingEnabled()` on; with them unset, billing and all entitlement gating are disabled.
Subscription state is synced **only** through the signature-verified webhook — the client is
never trusted for billing status.

**Gotchas.** A wrong `STRIPE_WEBHOOK_SECRET` makes the webhook silently fail signature
verification, so subscriptions never sync and there's no client-visible error — test it (§13).
`APP_BASE_URL` (set in §8) is where Stripe Checkout and the Billing Portal redirect back, so it
must be the frontend origin.

## 11. Frontend on Amplify

**Connect the repo.**
What: Amplify Hosting → host a web app → connect the GitHub repo. Set the app root to
`frontend/`, build command `npm run build`, output directory `dist/`.

**Set the build-time API URL.**
What: in the Amplify app's environment variables, set `VITE_API_URL=https://api.chillcheck.app`.
Why: this is a **build-time** value. `frontend/vite.config.ts` derives the Content-Security-
Policy `connect-src` from `VITE_API_URL`; if it's wrong or missing, the browser blocks the
app's API calls. The CSP is also the real mitigation for the JWT living in localStorage, so it
must point at the true API origin.

**Add the custom domain.**
What: Amplify → Domain management → add `app.chillcheck.app`; Amplify creates the Route 53
records and a managed cert. Record the app id.

**Why this choice.** The frontend is a static bundle, so Amplify hosts it cheaply, rebuilds on
push, and manages TLS and the CDN.

**Gotchas.** A blank dashboard with `connect-src` violations in the browser console means
`VITE_API_URL` was wrong **at build time** — fix the variable and trigger a redeploy
(re-running the build). If you ever change the API domain, you must rebuild the frontend, not
just repoint DNS.

## 12. Gateway on a Raspberry Pi

**Create a gateway and its key.**
What: in the deployed app, open a location's gateway setup and create a gateway (the API route
is `POST /api/locations/{id}/gateways`). The plaintext key (`chk_gw_…`) is shown **once** —
record it for the Pi. Bind each sensor to a unit by setting the unit's `sensor_mac` (uppercase
colon form, e.g. `A4:C1:38:...`).
Why: the gateway authenticates with this key on a separate auth path (`X-Gateway-Key`);
readings arrive keyed by MAC and only known MACs are stored.

**Cross-compile for the Pi (arm64).**
What:

```bash
cd gateway && CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o chillcheck-gateway . && cd ..
```

Why standard Go (not TinyGo): `tinygo.org/x/bluetooth` is a normal Go module that uses BlueZ
over D-Bus on Linux; the agent needs a real OS (a Pi or mini-PC), not a microcontroller.

**Install on the Pi.**
What: copy the binary to `/opt/chillcheck-gateway/`, create `config.yaml`:

```yaml
api_url: https://api.chillcheck.app
gateway_key: chk_gw_...        # or omit and set CHILLCHECK_GATEWAY_KEY in the unit
sample_interval: 5m
spool_path: /opt/chillcheck-gateway/spool.jsonl
spool_max: 20000
```

Install the unit from `gateway/systemd/chillcheck-gateway.service`, then:

```bash
sudo systemctl enable --now chillcheck-gateway
sudo systemctl status chillcheck-gateway
```

**Why the spool matters.** The agent store-and-forwards readings to disk during outages, so the
compliance record stays gap-free; buffered sensor readings keep their real measurement
timestamp when they're finally delivered.

**Gotchas.** BLE scanning needs `CAP_NET_ADMIN`/`CAP_NET_RAW` (the unit grants them) or root.
The unit runs as `User=chillcheck`, so that user must exist on the Pi (or change it). To keep
the key out of the config file, drop it and set `Environment=CHILLCHECK_GATEWAY_KEY=chk_gw_...`
in the unit instead.

## 13. Smoke test

Run these against the live stack, in order — each line proves a specific piece works:

1. **Register** an org + admin at `https://app.chillcheck.app`. → frontend ↔ API ↔ RDS path is good.
2. **Add a location and a unit.** → authed writes and org scoping work.
3. **Log a manual temp.** → the reading chain accepts a write.
4. **Open the compliance PDF** and check it prints **integrity VERIFIED**. → the hash chain
   is intact end-to-end.
5. **Force an out-of-range reading** and confirm an **alert email arrives**. → SES + the
   in-process alert engine + admin-recipient lookup all work together.
6. **`GET https://api.chillcheck.app/api/integrity`** returns OK. → tamper-evidence endpoint live.
7. **(Stripe)** start checkout as the admin and confirm the subscription syncs back. → the
   webhook signature + `STRIPE_WEBHOOK_SECRET` are correct.
8. **(Gateway)** with the Pi running, confirm a `source='sensor'` reading appears on the board. →
   the ingest path + gateway key + MAC binding work.

## 14. Cost estimate and teardown

**Rough monthly pilot cost** (order of magnitude — check current AWS/Stripe pricing):

- App Runner: smallest config, a few dollars/month idle, scales with traffic.
- RDS `db.t4g.micro` single-AZ: low-tens of dollars/month (the main steady cost).
- Amplify Hosting: cents-to-low-dollars at pilot traffic.
- Route 53: ~$0.50/month per hosted zone + the annual `.app` registration.
- SES: effectively free at pilot volume.
- Secrets Manager: ~$0.40/secret/month.
- Stripe: per-transaction, no fixed fee.

**Teardown** (reverse of build, so nothing is orphaned): delete the App Runner service → the
VPC connector → the RDS instance (decide on a final snapshot) → the Amplify app → the ECR repo
→ the Secrets Manager secrets → the SES identity → the Stripe webhook. Keep or release the
domain as you choose.
Why document teardown: the resources that quietly cost money are the hourly-billed ones (RDS
above all). Tearing down in this order leaves nothing running and nothing depending on a
deleted resource.

## 15. Codify in Terraform later

This runbook is deliberately manual: doing the deploy by hand once teaches what each resource
is and how they connect, and that intuition is what makes infrastructure-as-code readable.

Once this setup is stable, it becomes the spec for a Terraform module — the resources, their
relationships, and the env/secret wiring are all written down here. At that point, also add a
GitHub Actions pipeline (build → push to ECR → trigger an App Runner deploy) so releases stop
being manual. Not yet, though: for a single pilot, IaC and CI are maintenance burden against a
setup that's still changing. YAGNI until there's a second environment or a team that needs
repeatable provisioning.
