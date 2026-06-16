# PHI-1A — Production Deployment — Design & Implementation

**Status:** Designed + implemented (deployable artifacts). No application feature code. PROJECT_STATE not updated. Not committed.
**Goal:** take PlayArena from the development `docker-compose.yml` to a **pilot-ready production deployment** — TLS, CDN, durable managed database, and real email — the deployment slice of PHI-1.
**Constraints honoured:** no new capabilities/personas/reputation/recruitment/FE-8D; `GP_PLAYER_PERSONA_ENABLED=false`.

This is **infrastructure**, so "implement" means committing the runnable deployment artifacts, not provisioning external SaaS (which is an operator action). What ships:

| Artifact | Purpose |
|---|---|
| `frontend/next.config.ts` → `output: "standalone"` | self-contained Next server bundle for a small prod image |
| `frontend/Dockerfile` | production web image (standalone, non-root, healthcheck) |
| `frontend/.env.production.example` | `NEXT_PUBLIC_API_URL` (build-time) |
| `backend/.env.production.example` | complete, validated against `config.validate()` |
| `deploy/docker-compose.prod.yml` | api + web + Caddy; managed PG external; obs internal-only |
| `deploy/Caddyfile` | origin TLS, reverse proxy, Cloudflare real-IP, security headers |
| `deploy/README.md` | step-by-step runbook (PG, email, Cloudflare, deploy, smoke) |
| `deploy/.gitignore` | keeps secrets/origin certs out of git |

---

## 1. Production architecture

```
client ──HTTPS──> Cloudflare (edge TLS · CDN for public pages · WAF · IP rate-limit)
                       │ HTTPS, Full(strict), Origin Cert
                  Caddy (origin)  ── derives real client IP from trusted CF headers,
                   │     │           adds security headers, terminates origin TLS
            api.*  │     │ app.*
              ┌────▼─┐ ┌─▼────┐
              │ api  │ │ web  │     Go API (:8080 public, :9090 internal-only),
              └──┬───┘ └──────┘     Next.js standalone (:3000)
                 │ TLS sslmode=require
        Managed Postgres (Neon/Supabase/RDS, PITR)
        Email: Resend/SES   ·   Media: Cloudflare R2 (or local volume)
```

**Single origin host, three containers** (api, web, Caddy) on a private bridge
network with a fixed subnet (`172.28.0.0/16`). Only Caddy publishes ports
(80/443). The API's two ports are **never published**: `8080` is reachable only
by Caddy on the private network; `9090` (observability: `/metrics`,`/ready`,`/live`)
is reachable only by the optional Prometheus container. Postgres is **external**
(managed) — it is not a container here, so backups/PITR are the provider's job.

**Why this shape for a grassroots pilot:** smallest footprint that is still
*trustworthy*. One VM keeps cost and ops trivial; Cloudflare gives TLS, a CDN for
the public surface, and DDoS/WAF for free; managed Postgres removes the single
largest data-loss risk. It scales to "one real tournament" without a Kubernetes
tax. (Single-instance only — the realtime SSE hub is in-process; do not
horizontally scale `api` during the pilot.)

**Request-path security invariants preserved:** the app's per-IP rate limiting
and audit logging depend on seeing the **real** client IP. The chain
Cloudflare → Caddy → API is configured so that: Cloudflare sets `CF-Connecting-IP`;
Caddy `trusted_proxies` lists Cloudflare ranges and re-stamps `X-Forwarded-For`
with the true client IP; the API's `TRUSTED_PROXY_CIDRS` trusts only the Caddy
subnet and rewrites `RemoteAddr` accordingly. Misconfiguring this would make
every request look like it came from Cloudflare (defeating per-IP limits) — the
runbook smoke test explicitly checks that logs show real client IPs.

**Config is fail-closed in production** (`config.validate()`): the API refuses to
boot unless `JWT_SECRET ≥ 32` chars (and ≠ default), `FRONTEND_URL` is `https://`,
`EMAIL_PROVIDER` is not `log`/`noop`, `WEBHOOK_SECRET_KEY`/`DATABASE_URL`/
`EMAIL_FROM_ADDRESS` are set, and `APP_INTERNAL_PORT ≠ APP_PORT`. The
`.env.production.example` is written to satisfy exactly these.

**Frontend build-time coupling:** Next inlines `NEXT_PUBLIC_API_URL` at build, so
it is a **build arg** (the public API origin). Server-side public-page fetches and
client-side calls both use it. CORS on the API must allow the web origin
(`CORS_ALLOWED_ORIGINS=https://app.<domain>`).

---

## 2. Cloudflare setup

Cloudflare is the edge: **TLS termination**, **CDN for the public surface**,
**WAF/DDoS**, and **IP-based rate limiting** that complements the app limiter
(critical because a whole venue typically shares one NAT IP).

- **DNS:** proxied (orange-cloud) `app.<domain>` and `api.<domain>` → origin IP.
- **SSL/TLS = Full (strict)** with a Cloudflare **Origin Certificate** mounted into
  Caddy (`deploy/certs/origin.{pem,key}`). Full(strict) means client↔edge and
  edge↔origin are both encrypted and the origin cert is validated — no plaintext hop.
  (HTTP-01 from Let's Encrypt won't work behind the proxy, hence the origin cert.)
- **Always Use HTTPS**, **Min TLS 1.2**, **HSTS** after verification.
- **CDN cache rule** for `app.<domain>/t/*`: eligible for cache, respect origin
  TTL — the public pages and the public API already emit `Cache-Control: public,
  max-age=20`, so a CDN absorbs spectator/scrape load and keeps public pages up
  even during an API restart. `_next/static/*` is immutable-cached automatically.
- **WAF rate-limit rule** on `api.<domain>/api/v1/auth/*` as defence-in-depth.
- **Simpler alternative:** **Cloudflare Tunnel** (`cloudflared`) to `api:8080`/
  `web:3000` removes inbound ports and the origin cert entirely — recommended if
  the VM has no static public IP.

---

## 3. Managed Postgres strategy

**Decision: managed Postgres 17 with automated backups + PITR — never a
self-hosted container for the pilot.** The database is the system of record for a
real event; losing it is unrecoverable, and a single Docker volume has no backup
story.

- **Provider:** Neon or Supabase (simplest, generous free/low tiers, fast setup)
  or AWS RDS / DO Managed PG if already in that ecosystem; region near the venue.
- **Connection:** `DATABASE_URL` with **`sslmode=require`** (encrypted in transit).
- **Backups/PITR:** rely on the provider's continuous WAL/PITR (RPO ≈ minutes,
  matching the PHI-1 live-tournament target) plus the provider's daily snapshots;
  optionally a nightly logical `pg_dump` to object storage as a portable
  belt-and-suspenders. A restore must be **rehearsed once** before the pilot (the
  backup/restore drill is PHI-1's next slice; PHI-1A makes the DB capable of it).
- **Migrations:** the one-shot `migrate` compose service applies `000001`–`000030`
  against `DATABASE_URL` before the app starts; deploys run it as a gate.
- **Why not the dev Postgres container:** it bundles credentials, has no PITR, and
  ties data durability to one host's disk — exactly the P0 risk PHI-1 flagged.

---

## 4. Email provider

Verification and password-reset email is the onboarding front door; in the dev
stack it goes to **MailHog**, which does not deliver to real inboxes.

- **Decision: Resend (recommended) via SMTP, or AWS SES.** Resend gives the
  fastest domain + DKIM setup and strong deliverability at pilot volume; SES is the
  pick if already on AWS (`ap-south-1` for India).
- **Deliverability is the point:** add **SPF, DKIM, and DMARC** records (the
  provider generates them) in Cloudflare DNS and confirm "Verified" before the
  pilot — unverified senders land in spam and silently break onboarding.
- **Config:** `EMAIL_PROVIDER=smtp` + `smtp.resend.com:587` + API-key password +
  `EMAIL_SMTP_TLS=true`, from-address on the verified domain. (SES variant is in
  the env example.) Production boot is blocked for `log`/`noop` providers.
- **Operational watch:** delivery is async via the `EmailWorker` draining pending
  rows; a misconfigured provider queues silently — PHI-1 adds a pending-email
  backlog alert. The PHI-1A smoke test verifies a real verification email arrives
  in an inbox.

---

## 5. Validation & smoke test

- Build/typecheck: `frontend` builds with `output: standalone` (validated);
  backend image is the existing multi-stage Dockerfile (non-root, `/live`
  healthcheck). No application code changed, so the FE-8/PRI-1 test suites are
  unaffected.
- Go/no-go smoke (full list in `deploy/README.md` §6): API reachable and `:9090`
  **not** publicly reachable; verification email lands in an inbox; neutral
  `/welcome` after login; a Public tournament link loads **logged out** and
  unfurls on WhatsApp; public pages survive an API restart (CDN); logs show real
  client IPs.

## 6. Out of scope (later PHI-1 slices)
Backup/restore *drill* + DR runbook, Alertmanager rules + external uptime +
error tracking, real-device scorer validation, and the concurrent-scorer
mitigation. PHI-1A delivers the deployment substrate those depend on.
