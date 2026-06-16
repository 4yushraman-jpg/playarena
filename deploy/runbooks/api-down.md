# Runbook — API Down

**Symptoms:** `APIUnavailable` alert (`up{job="playarena_api"}==0`); external
uptime monitor red on `api.<domain>`; users can't log in/score; public pages may
still load (CDN-cached).

**Diagnosis**
```sh
docker compose -f deploy/docker-compose.prod.yml ps          # is `api` running?
docker compose -f deploy/docker-compose.prod.yml logs --tail=200 api
docker exec <api> wget -qO- http://localhost:9090/live       # internal liveness
```
- Crash loop in logs → bad config/secret or a panic. Config validation failures
  print exactly which env var is wrong (`config validation failed: …`).
- `live` ok but unreachable externally → Caddy / Cloudflare issue (see public-outage).
- DB errors in logs → see database-down.md.

**Immediate actions**
- Container stopped/crashed: `docker compose -f deploy/docker-compose.prod.yml up -d api`.
- Bad config after a change: fix `backend/.env.production`, recreate the container.
- Post-deploy regression: **rollback** (rollback.md).

**Recovery actions**
- Confirm it stays up (no crash loop) for 2 min; check 5xx rate on the Pilot Day
  dashboard returns to baseline.

**Escalation**
- If it crash-loops on a config/secret you can't resolve, or the host itself is
  down → platform owner; consider environment rebuild (restore.md "Full rebuild").

**Verification**
- `APIUnavailable` resolves; `/live` green; a login succeeds; a live match can be
  scored; scorers' queued events reconcile (no data lost across the restart).
