# Runbook / Setup — External Uptime Monitoring

**Why external:** the in-process `/metrics` and Prometheus run on the same host as
the API — if the host dies, they die too and can't tell you. An **independent**
monitor is the only thing that detects a total outage.

## Choice: UptimeRobot (free tier)
Evaluated: UptimeRobot, Better Stack, Pingdom, Cloudflare Health Checks.
**Pick UptimeRobot** for the pilot — free, 1-minute checks, email/SMS/Slack/
Telegram/webhook alerts, zero infra. (Better Stack is a fine paid upgrade with
status pages; Cloudflare Health Checks need a paid plan; Pingdom is overkill/paid.)

## Monitors to configure (HTTP(S), 1-minute interval, alert after 1 failure)
1. **API** — `https://api.<domain>/api/v1/...` *(a lightweight 200 path; do NOT
   point it at the internal `:9090`, which is private)*. Keyword/health endpoint.
2. **Frontend** — `https://app.<domain>/login` → expect 200.
3. **Public tournament page** — a known **public** tournament:
   `https://app.<domain>/t/<org>/<tournament>` → expect 200 and a keyword (e.g.
   the tournament name) so a blank/error page is caught, not just the status code.

## Alert contacts
- Add the on-call email **and** a Slack/Telegram webhook (same channel as
  Alertmanager) so external + internal alerts land together.
- Optionally enable **SMS** for the API monitor only (tournament day).

## Why three monitors
Each isolates a layer: API monitor = backend/origin; frontend monitor = web
container/Cloudflare; public-page monitor = the end-to-end spectator path
(Cloudflare → web → public API → DB). The public-page keyword check is the truest
"can a parent see the bracket right now?" signal.

## Verification
- Trigger a deliberate failure (stop `web`) → the frontend + public-page monitors
  go red and alert within ~1–2 min → restart → they recover. Document the observed
  detection time.
