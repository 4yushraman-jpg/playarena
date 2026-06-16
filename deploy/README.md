# PlayArena — Production Deployment Runbook (PHI-1A)

Takes PlayArena from the dev `docker-compose.yml` to a **pilot-ready production
deployment**: Cloudflare in front, a single origin host running Caddy + the API
+ the Next.js web app, a **managed Postgres** (not self-hosted), and a real email
provider. Architecture rationale: `docs/architecture/phi-1a-production-deployment.md`.

```
                 client
                   │  HTTPS
            ┌──────▼───────┐   TLS edge · CDN (public pages) · WAF · IP rate-limit
            │  Cloudflare  │
            └──────┬───────┘   HTTPS (Full strict, Origin Cert)
            ┌──────▼───────┐
            │  Caddy (origin)  reverse proxy · real client IP · sec headers
            └───┬───────┬──┘
        api.* │       │ app.*
        ┌─────▼──┐  ┌─▼─────────┐
        │  api   │  │   web      │   (Go API :8080 / :9090-internal, Next :3000)
        └───┬────┘  └───────────┘
            │ TLS (sslmode=require)
   ┌────────▼─────────┐
   │ Managed Postgres │  (Neon / Supabase / RDS) — PITR backups
   └──────────────────┘
   Email: Resend/SES   ·   Media: Cloudflare R2 (or local volume)
```

---

## 0. Prerequisites
- A domain (e.g. `playarena.app`) on **Cloudflare** (nameservers delegated).
- A small Linux VM (2 vCPU / 4 GB is plenty for a pilot) with Docker + Compose.
- A **managed Postgres** instance with automated backups + PITR.
- An email provider account (**Resend** recommended) with the sending domain verified.

---

## 1. Managed Postgres
**Do not run Postgres in a container for the pilot** — use a managed service so
backups/PITR are not your problem.

1. Create a Postgres 17 instance (Neon / Supabase / RDS / DO Managed PG), region
   close to the venue (e.g. `ap-south-1` for India).
2. Confirm **automated daily backups + Point-In-Time-Recovery (WAL)** are enabled
   (RPO ≈ minutes — see the backup/RTO targets in PHI-1).
3. Copy the connection string and append **`?sslmode=require`** →
   `DATABASE_URL` in `backend/.env.production`.
4. Create the schema:
   ```sh
   export DATABASE_URL='postgres://USER:PASS@HOST:5432/playarena?sslmode=require'
   docker compose -f deploy/docker-compose.prod.yml run --rm migrate
   ```
   (Runs migrations `000001`–`000030`.)

## 2. Email provider
1. **Resend** (recommended): add the domain, create the DNS records it gives you
   (SPF, DKIM, DMARC) **in Cloudflare DNS**, wait for "Verified", create an API key.
2. In `backend/.env.production` set `EMAIL_PROVIDER=smtp`, `EMAIL_SMTP_HOST=smtp.resend.com`,
   `EMAIL_SMTP_PORT=587`, `EMAIL_SMTP_USERNAME=resend`,
   `EMAIL_SMTP_PASSWORD=<API key>`, `EMAIL_SMTP_TLS=true`, and a from-address on
   the verified domain. (SES alternative is commented in the env example.)
3. The app **refuses to boot in production** with `EMAIL_PROVIDER=log/noop`.

## 3. Secrets & env
```sh
cp backend/.env.production.example backend/.env.production
# Fill: DATABASE_URL, JWT_SECRET (openssl rand -base64 48), WEBHOOK_SECRET_KEY
# (openssl rand -base64 32), FRONTEND_URL=https://app.<domain>,
# CORS_ALLOWED_ORIGINS=https://app.<domain>, email creds.
cp frontend/.env.production.example frontend/.env.production   # NEXT_PUBLIC_API_URL=https://api.<domain>
```
`deploy/.gitignore` keeps `.env*` and `certs/` out of git. Prefer the host's
secret store / `chmod 600` over plaintext where possible.

## 4. Cloudflare
1. **DNS:** add proxied (orange-cloud) `A`/`AAAA` records `app.<domain>` and
   `api.<domain>` → the origin VM IP.
2. **SSL/TLS mode:** set to **Full (strict)**. Create an **Origin Certificate**
   (Cloudflare → SSL/TLS → Origin Server), save the cert/key to
   `deploy/certs/origin.pem` and `deploy/certs/origin.key`.
3. **Always Use HTTPS:** on. **Min TLS 1.2.** Enable **HSTS** (after verifying).
4. **CDN for public pages:** add a Cache Rule — for `app.<domain>/t/*` *Eligible
   for cache* + *Respect origin* TTL (the public API and pages already send
   `Cache-Control`). Static `_next/static/*` is cached automatically.
5. **WAF / rate limiting (optional but recommended):** a rate-limit rule on
   `api.<domain>/api/v1/auth/*` complements the app's per-IP limiter (important
   because a whole venue often shares one NAT IP).
6. Update `deploy/Caddyfile` hostnames (`api.example.app` / `app.example.app`).
   - *Simpler alternative:* skip the origin cert and inbound ports by running
     **Cloudflare Tunnel** (`cloudflared`) to `api:8080` and `web:3000`.

## 5. Deploy
```sh
cd deploy
# Build images (NEXT_PUBLIC_API_URL is baked into the web build):
NEXT_PUBLIC_API_URL=https://api.<domain> docker compose -f docker-compose.prod.yml build
docker compose -f docker-compose.prod.yml up -d
# Optional dashboards (kept on the private network; reach via SSH tunnel):
GRAFANA_ADMIN_PASSWORD=... docker compose -f docker-compose.prod.yml --profile observability up -d
```

## 6. Smoke test (go/no-go)
- `https://api.<domain>/api/v1/...` reachable; **`:9090` NOT reachable publicly**.
- Register → **verification email arrives in the inbox** (not spam) → log in →
  land on `/welcome` (neutral onboarding) → create org.
- Create a tiny tournament → set visibility **Public** → open
  `https://app.<domain>/t/<org>/<tournament>` **logged out** → fixtures/standings load.
- Share the link in WhatsApp → it **unfurls** (OG card).
- Restart the API container → public pages **stay up** (served from Cloudflare cache).
- `docker compose logs api` shows real client IPs (not Cloudflare IPs) → trusted-proxy chain OK.

## 7. Operational notes
- Observability port `9090` is never published; Prometheus scrapes it on the
  private docker network. Reach Grafana via SSH tunnel, not a public hostname.
- `TRUSTED_PROXY_CIDRS=172.16.0.0/12` covers the compose network `172.28.0.0/16`
  so the API derives the real client IP from Caddy's `X-Forwarded-For`.
- Media: `STORAGE_BACKEND=local` uses the `uploads_data` volume (back it up); for
  durability switch to **Cloudflare R2** (`STORAGE_BACKEND=s3`, see env example).
- Single instance only (the realtime SSE hub is in-process). Do not scale `api`
  horizontally during the pilot.
