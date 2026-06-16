# Staging deploy — Cloudflare Tunnel on a VM

Goal: a stable, phone-reachable HTTPS staging URL so the device/network matrix
(`pilot-validation-protocol.md`) can be run. Topology: Cloudflare edge →
`cloudflared` (outbound tunnel) → `web` + `api` containers; managed Postgres.
No static IP, no open ports, no origin certificate.

Compose file: `deploy/docker-compose.staging.yml`.

---

## 0. What you provision (one-time)

| Thing | Where | Output you need |
|---|---|---|
| Domain | any registrar, nameservers → Cloudflare | `app.<domain>`, `api.<domain>` |
| Cloudflare account | dash.cloudflare.com (free) | zone for the domain |
| Managed Postgres | Neon (free tier) | `DATABASE_URL` (with `sslmode=require`) |
| VM | Hetzner CX22 / DO basic (~$5/mo), Ubuntu + Docker | SSH access |
| Email (optional) | Resend | SMTP API key — OR skip (see §4) |

Secrets already generated for you (regenerate with `openssl rand -base64 48` /
`... 32` if you prefer): keep `JWT_SECRET` and `WEBHOOK_SECRET_KEY` private.

---

## 1. VM prep
```bash
ssh root@<vm-ip>
curl -fsSL https://get.docker.com | sh           # Docker + compose plugin
git clone <repo> playarena && cd playarena/deploy
```

## 2. Postgres (Neon)
Create a project (region near you), copy the connection string, ensure it ends
with `?sslmode=require`. That's `DATABASE_URL`.

## 3. Cloudflare Tunnel
1. Zero Trust → Networks → **Tunnels → Create a tunnel** (type: *Cloudflared*).
2. Copy the **tunnel token** → `CLOUDFLARE_TUNNEL_TOKEN`.
3. Add two **public hostnames** to the tunnel:
   - `app.<domain>` → service `http://web:3000`
   - `api.<domain>` → service `http://api:8080`
   (cloudflared resolves `web`/`api` because it's on the same compose network.)
4. DNS records are created automatically by the tunnel (CNAME → tunnel).

## 4. Email — pick one
- **Real inboxes (closest to pilot):** Resend → verify the domain (SPF/DKIM/DMARC
  records into Cloudflare DNS) → put the SMTP API key in `.env.staging`
  (`EMAIL_PROVIDER=smtp`, `smtp.resend.com:587`).
- **Skip email for staging:** set `APP_ENV=staging` in `.env.staging`. Config
  validation only enforces a real provider when `APP_ENV=production`, and
  registration returns `verification_token` in the response so testers verify
  without an inbox. Faster to stand up; switch to Resend before the real pilot.

## 5. Env file
```bash
cp ../backend/.env.production.example ../backend/.env.staging
```
Edit `../backend/.env.staging` and set at minimum:
- `APP_ENV=staging` (or `production` if using Resend with a verified domain)
- `DATABASE_URL=...?sslmode=require`
- `JWT_SECRET=` / `WEBHOOK_SECRET_KEY=` (the generated values)
- `FRONTEND_URL=https://app.<domain>`
- `CORS_ALLOWED_ORIGINS=https://app.<domain>`
- `TRUSTED_PROXY_CIDRS=172.16.0.0/12` (covers the 172.28.0.0/16 compose subnet)
- email vars per §4

## 6. Migrate + deploy
```bash
export DATABASE_URL="postgres://...sslmode=require"
export CLOUDFLARE_TUNNEL_TOKEN="..."
export NEXT_PUBLIC_API_URL="https://api.<domain>"

docker compose -f docker-compose.staging.yml run --rm migrate     # applies 000001..000030
docker compose -f docker-compose.staging.yml up -d --build
docker compose -f docker-compose.staging.yml ps                    # api+web+cloudflared healthy
```

## 7. Smoke test (go/no-go)
- `https://app.<domain>` loads; `/login`, `/register` work.
- Register → verify (real email, or token from response) → login → lands on `/welcome`.
- Create org → tournament → fixtures → score → complete (organizer happy path).
- A public tournament link loads **logged out** and on a phone; sub-tabs
  (Fixtures/Bracket/Standings/Results) render.
- `https://api.<domain>/api/v1/health` → 200; the observability port (9090) is
  **not** reachable publicly.
- API logs show **real client IPs** (not the cloudflared container IP) — confirms
  `TRUSTED_PROXY_CIDRS` is right.
- Then run `deploy/runbooks/pilot-validation-protocol.md` on real devices.

## Rollback / ops
- Logs: `docker compose -f docker-compose.staging.yml logs -f api`
- Update: `git pull && docker compose -f docker-compose.staging.yml up -d --build`
- Stop: `docker compose -f docker-compose.staging.yml down`
- DB is managed → backups/PITR are Neon's; still rehearse `restore-drill.sh`.
