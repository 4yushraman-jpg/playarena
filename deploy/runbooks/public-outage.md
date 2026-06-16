# Runbook — Public Pages Outage

Public tournament pages (`app.<domain>/t/...`) are what spectators/parents open
from a share link. Served by the `web` (Next.js) container, fed by the public API,
fronted by Cloudflare (which caches them).

**Symptoms:** `PublicPageErrors` alert; uptime monitor red on the public page URL;
organizer reports "the link is broken / blank".

**Diagnosis — narrow the layer**
```sh
# Origin web container up?
docker compose -f deploy/docker-compose.prod.yml ps web
docker compose -f deploy/docker-compose.prod.yml logs --tail=200 web
# Public API healthy? (the pages fetch it)
curl -s -o /dev/null -w '%{http_code}\n' https://api.<domain>/api/v1/public/orgs/<org>/tournaments/<t>
```
- **404 on a page the organizer expects public** → not an outage: the tournament
  is `private`/`draft`. Have them set visibility to **Unlisted/Public** in the
  Share control. (This is the most common "outage" report.)
- API 5xx → see api-down/database-down. web container down → restart it.
- Origin fine but Cloudflare error page → Cloudflare/edge or origin-cert issue.

**Immediate actions**
- web down: `docker compose -f deploy/docker-compose.prod.yml up -d web`.
- Cloudflare edge issue: check Cloudflare status; verify DNS proxied + SSL mode
  Full(strict) + origin cert valid; purge cache if serving a stale error.
- API-caused: resolve via api-down/database-down — note public pages stay up from
  CDN cache during a brief API blip.

**Escalation**
- Cloudflare incident → status page; if prolonged, temporarily set DNS to
  "DNS only" (grey cloud) to bypass the edge (loses CDN/WAF but restores access).

**Verification**
- Open the public link **logged out**; fixtures/standings render; `PublicPageErrors`
  resolves; link still unfurls (OG) in a chat app.
