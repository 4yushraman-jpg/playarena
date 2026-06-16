# PHI-1 — Production Pilot Hardening — Readiness Review

**Status:** Analysis only. No code, no implementation, no PROJECT_STATE change.
**Question:** *If a district-level Kabaddi organizer runs a tournament next weekend, what could fail, and how do we eliminate it?*
**Mission:** Make PlayArena **operationally trustworthy** — no new capabilities, personas, reputation, recruitment, or FE-8D. GP-2 stays OFF.

**Headline verdict:** The *product* is pilot-capable; the *operation* is not. The application layer (correctness, isolation, offline scoring, public surface) is strong and tested. The gaps are almost entirely **operational** — no backups, no validated production deployment, unverified email, no alerting/runbooks, and the scorer never tested on a real phone. PHI-1 is ~80% infrastructure/process and ~20% small code hardening.

---

## 1. Tournament Director Simulation

For each: organizer → scorer → spectator, with friction/failure/support-request callouts.

**8 / 16 / 32-team knockout.**
- *Organizer:* create → generate (wizard) → **must manually flip the tournament to `ongoing`** before any match can start → run. **Friction F1 (P1):** the "set to Ongoing" step is implicit; a director will try to start a match and hit `ErrTournamentNotOngoing` → support: *"why can't I start the match?"*. **Friction F2 (P1):** generated matches have **no `scheduled_at`/venue** — for 32 teams across a day there's no publishable schedule; the public "Fixtures" tab shows times as blank. **F3 (P2):** the 32-team public bracket is a wide horizontal scroll on a phone.
- *Scorer:* opens the match scorer, scores live. Offline-tolerant + exactly-once is solid. **Failure point FP1 (P0/P1):** if two volunteers open the same match (common at a busy venue), both can write — only a frontend *awareness banner* exists, no backend lease → divergent event logs → corrupted result.
- *Spectator:* needs the share link. **Friction F4 (P1):** visibility defaults to **private**; if the organizer forgets to set Unlisted/Public, *"my players can't open the link"* → support. Knockout TBD slots show "TBD" until feeders finish (expected, mild confusion F5/P2).

**12-team league.** 66 matches, 11 rounds. *Organizer:* generate → ongoing → score 66 matches. **Friction F6 (P1):** no scheduling means 66 timeless fixtures; the "what's next / which court" question is unanswered. Standings update only on completion (correct). Spectators get a clean standings table (good).

**24-team group + knockout.** Group stage (24 matches) → **must click "Resolve qualifiers"** after the group stage → knockout. **Friction F7 (P1):** resolve-qualifiers discoverability — the action only appears when the group stage is complete; a director who doesn't notice it sees a permanently "TBD" knockout → support. Otherwise the flow is sound.

**Cross-cutting support magnets:** (a) lifecycle steps that are implicit (→ ongoing, resolve qualifiers); (b) no schedule/venue on generated fixtures; (c) private-by-default visibility; (d) **accidental walkover/abandon is terminal with no organizer undo** → *"I clicked walkover by mistake"* needs an operator correction path.

---

## 2. Operational Risk Review

| Area | Sev | Risk |
|---|---|---|
| **Backups / Restore** | **P0** | No automated backups, no restore procedure. This is the system of record for someone's real event — a DB loss is unrecoverable today. |
| **Email deliverability** | **P0** | `ses`/`smtp` supported but the pilot domain's SES/DKIM/SPF is unverified. Verification + password-reset are the front door; silent spam-foldering or auth failure blocks onboarding. Async `EmailWorker` queues rows — misconfig fails *silently* (no backlog alert). |
| **Production deployment** | **P0** | `docker-compose.yml` is a **dev** stack (bundles MailHog/Prometheus/Grafana on one host, no TLS, no secrets management, no restart policy). There is no validated production manifest. |
| **TLS** | **P0** | App terminates plain HTTP; production needs a TLS-terminating reverse proxy/CDN. A public, link-shared site over HTTP is unacceptable. |
| **Public page availability** | **P1** | Public pages are served by the API process (cacheable headers, but no CDN/origin separation). An API restart mid-event takes public pages down. |
| **Monitoring / Alerting** | **P1** | Prometheus metrics + `/live`/`/ready` exist, but **no alert rules, no Alertmanager, no external uptime check** → an outage is discovered by users, not operators. |
| **Logging / retention** | **P1** | Structured slog to stdout; no aggregation/retention/search → post-incident debugging is hard. |
| **Secrets management** | **P1** | JWT secret, SES keys, webhook key via env — needs a real secret store / `.env` discipline in prod (not committed, rotated). |
| **Concurrent scorers** | **P1** | Awareness-only; no lease (see §3). |
| **Rate limiting** | **P2** | Present (auth/write/media/public). Tune thresholds for venue NAT (many users behind one IP) so spectators aren't throttled. |
| **Realtime SSE** | **P2** | In-process hub → single-instance only. Fine for a single-instance pilot; do **not** horizontally scale during it. |

---

## 3. Live Match Reliability Review

**Strong (verified by design/tests):** offline-first queue with `client_event_id`, reconcile-before-resend (exactly-once), server-authoritative score, completion gate. Refresh, reconnect, and intermittent network are handled — unsynced events persist in `localStorage` and reconcile on return.

**What could still corrupt trust:**
- **Concurrent scorers (P1, the #1 risk).** Two authorized scorers on one match both write; only the frontend warns. Mitigations, in order: (a) **process** — assign exactly one scorer per match (runbook + briefing); (b) make the awareness banner unmissable; (c) the only *code* fix is a backend scorer-lease — that borders on a feature, so for the pilot prefer (a)+(b) and document it.
- **Accidental walkover / abandon (P1).** Terminal, no organizer undo. Needs an operator correction runbook (platform-admin SQL) and a confirm-friction check in the UI flow.

**What could still confuse scorers:**
- **Device sleep / battery saver (P1, must validate).** Background timer throttling can drift the advisory clock and pause SSE; on resume the score must re-fetch and the queue must flush. *Behavior is designed for this but never tested on a real phone.*
- Re-opening a mid-match scorer starts the advisory clock at Half 1 (known FE-7 carry-over) — cosmetic, but brief the scorer.

**Harden before pilot:** real-device validation (§7); the concurrent-scorer process + banner; a walkover/abandon correction runbook.

**Tournament-day resilience (already true, must be documented):** if the API crashes mid-match, scorers' unsynced events survive in the browser queue and reconcile on restart → **no scoring data lost** (RPO ≈ 0 for in-flight events from the client side). This is a genuine strength to lean on.

---

## 4. Production Environment Recommendation (minimum)

A grassroots pilot needs the smallest *trustworthy* footprint, not a platform:

- **Compute:** one small VM (Hetzner CX22 / DigitalOcean / Fly.io / Render) running the API container + reverse proxy. PaaS (Render/Railway/Fly) is fine and reduces ops.
- **Database:** **managed Postgres with automated backups + PITR** — Neon or Supabase (simplest, free/low tier, India-friendly) or AWS RDS / DigitalOcean Managed PG. **Do not self-host Postgres in a single container for the pilot.**
- **Email:** **Resend or Postmark** (fastest domain+DKIM setup, excellent deliverability for low volume) — preferred over SES for a first pilot unless already on AWS (then SES `ap-south-1`). Verify SPF/DKIM/DMARC before the pilot.
- **Domain + TLS + CDN:** a domain behind **Cloudflare** (free): TLS, CDN caching for public pages (honors the existing `Cache-Control`), DDoS/WAF, and IP-aware rate limiting. Alternative: Caddy for auto-TLS if no CDN.
- **Monitoring:** Prometheus + Grafana (already in compose) on the host, **or Grafana Cloud free tier**; add **Alertmanager** → email/Telegram/Slack.
- **Uptime/alerting:** external monitor (UptimeRobot / Better Uptime) hitting `/live`; optional **Sentry** for FE+BE error tracking.

Minimal viable stack: **VM (API+Caddy) + Cloudflare + managed Postgres (PITR) + Resend + Grafana Cloud + UptimeRobot.**

---

## 5. Backup & Recovery Plan

**Targets (grassroots-appropriate):**
- **RPO ≤ 5 min during a live tournament** (continuous WAL/PITR) — losing more than a few minutes of live scoring is unacceptable mid-event; ≤ 24 h otherwise.
- **RTO ≤ 15 min** to restart the app on a fresh host; **≤ 60 min** for a full DB restore.

**Backup strategy:** managed-PG automated daily snapshot **+ PITR (WAL)** for minute-level RPO; **plus** a nightly logical `pg_dump` to object storage (portable, provider-independent belt-and-suspenders). If media uploads are used, enable object-store versioning / volume snapshots.

**Restore strategy:** documented, **rehearsed** steps for (a) PITR to a timestamp, (b) `pg_dump` restore to a new instance, then point the app at it. *An untested restore does not exist — run one restore drill before the pilot.*

**Disaster recovery:** VM dies → redeploy container, repoint to managed PG (data safe). Managed PG dies → PITR-restore to a new instance, repoint app. Region outage → restore dump in another region (accepted longer RTO for grassroots).

**Tournament-day recovery (fast path):** API crash → restart; scorers' offline queues reconcile (no loss). DB corruption mid-event → PITR-restore to latest (RPO minutes); at most a few minutes of scoring re-entered from scorers who still hold queued events.

---

## 6. Runbook Requirements

Concise, one-page each:
1. **API down** — check `/live`, container logs, restart; verify reverse proxy; confirm public pages via CDN cache.
2. **Database unavailable** — check managed-PG status, connection pool metrics; failover/PITR; app auto-reconnect behavior.
3. **Email outage** — check provider dashboard + `EmailWorker` pending-row backlog metric; resend; fall back to manual link sharing for organizers.
4. **Scorer issue** — lost/dead device (another scorer opens the match; queue is per-device so brief them); **double-scorer** (designate one, others close the tab).
5. **Accidental walkover / abandon** — operator correction procedure (platform-admin, documented SQL to revert a terminal match + recompute) — terminal states have no UI undo.
6. **Tournament misconfiguration** — wrong format/fixtures: regenerate **before** any match starts (generation blocks once fixtures exist); after play, manual correction.
7. **Public page outage** — distinguish CDN vs origin; purge cache; confirm visibility setting (private looks like an outage to the organizer).
8. **Restore-from-backup** — the rehearsed PITR/dump steps from §5.
9. **Deploy / rollback** — image tag, migrate, health-gate, rollback to previous tag.

---

## 7. Real-Device Validation Plan (scorer = mission critical)

**Matrix:** {Android·Chrome, iPhone·Safari} × scenarios. Pass criteria explicit.

| Scenario | Procedure | Pass criterion |
|---|---|---|
| Low bandwidth | throttle to 2G/3G; score 10 events | events appear optimistically; sync drains; no loss |
| Offline | airplane mode; score 10 events; re-enable | all 10 reconcile exactly once (no dup/no loss) |
| Reconnect | drop/restore wifi mid-raid | queue flushes; server score matches |
| Battery saver | enable; score for 5 min | events still sync; advisory clock may drift (acceptable; score authoritative) |
| Screen lock | lock during a raid; unlock | state restored; no loss on resume |
| Sleep/resume | background the tab 10 min; return | SSE reconnects, score re-fetches, queue flushes |
| Refresh | reload mid-match | scoreboard + pending queue restored |
| Backgrounding | switch apps, return | no duplicate submission |

Plus: **public pages at 320 px** (Android/iPhone), **WhatsApp link unfurl** (OG card renders), keyboard/screen-reader spot check (WCAG AA). Capture a signed-off checklist; the scorer scenarios are **go/no-go**.

---

## 8. Observability Review

**Have:** Prometheus HTTP metrics (count/latency/in-flight), per-limiter metrics (auth/write/media/public), DB-pool + outbox scrapers, SSE hub metrics, `/live`/`/ready`/`/metrics`, structured slog; Grafana in compose.

**Gaps → recommend:**
- **Alerting (P1):** no rules/Alertmanager. Add alerts: API `/live` down, 5xx rate, p99 latency, **DB pool saturation**, **pending-email backlog**, **webhook delivery failures**, rate-limit rejection spikes.
- **Business metrics (P1):** live-match count, scoring events/min, generations, walkovers, completions — so an operator can *see* the tournament happening.
- **External uptime (P1):** independent monitor on `/live` (the in-process metrics can't report that the process is down).
- **Error tracking (P2):** Sentry (FE + BE) for stack-traced exceptions a metric won't surface.
- **Log aggregation (P2):** ship slog to Grafana Loki / a hosted log service with ≥7-day retention.
- **Dashboards (P1):** "Pilot Day" board (API health + live activity + email/webhook delivery + DB) on one screen.

---

## 9. Pilot Readiness Score

| Dimension | Rating | Rationale |
|---|---|---|
| **Architecture** | 🟢 Green | Sound, modular, well-tested; correct isolation and public-path separation. |
| **Reliability (scoring)** | 🟡 Amber | Offline/exactly-once is strong; concurrent-scorer + real-device validation outstanding. |
| **Operations** | 🔴 Red | No backups, no validated prod deploy/TLS, unverified email, no runbooks. |
| **Security** | 🟡 Amber | Strong app-level (RBAC, BOLA, rate limits, public isolation); needs TLS, secrets hygiene, prod hardening. |
| **Usability** | 🟡 Amber | Deep but unproven; implicit lifecycle steps, no fixture scheduling, private-by-default. |
| **Observability** | 🟡 Amber | Good metrics; no alerting/uptime/error-tracking. |

**Overall: 🔴→🟡, NOT pilot-ready.** Operations must reach 🟢 (backups + restore drill + prod deploy + TLS + verified email) before any real tournament. That single dimension is the gate.

---

## 10. PHI-1 Implementation Plan

**Objectives:** make PlayArena operationally trustworthy for one real tournament — recoverable, deployable, observable, deliverable email, with runbooks and a validated scorer on real devices. **No new product capability.**

**Scope:**
1. **Backups + tested restore** (managed PG PITR + nightly dump; one rehearsed restore drill).
2. **Production environment** (VM/PaaS + reverse proxy/TLS + Cloudflare CDN for public pages + secrets).
3. **Email deliverability** (Resend/Postmark or SES; SPF/DKIM/DMARC verified; end-to-end verification + reset test).
4. **Observability** (Alertmanager + alert rules, external uptime, a Pilot-Day dashboard; optional Sentry).
5. **Runbooks** (§6) including the **walkover/abandon correction** procedure.
6. **Real-device validation** (§7 matrix, signed off).
7. **Concurrent-scorer mitigation** (process + prominent banner; backend lease explicitly deferred).
8. **Conditional, only if the pilot is multi-day:** bulk fixture scheduling (times/venues) — the one borderline code item; otherwise deferred.

**Out of scope:** GP-2/personas/profiles, reputation, recruitment, rankings, public player pages, FE-8D, any new feature.

**Files likely affected (mostly NON-code):**
- New: `docker-compose.prod.yml` (or PaaS config), reverse-proxy/Caddy config, `deploy/` manifests, `.env.production.example`, `prometheus/alerts.yml`, Grafana dashboard JSON, `docs/runbooks/*.md`, `docs/ops/backup-restore.md`, device-validation checklist.
- Small code (only if justified): a `pending_email` backlog gauge + webhook-failure metric for alerting; a UI nudge to set status `ongoing` / visibility after generation; (conditional) scheduling.

**Operational tasks:** provision managed PG + PITR; configure backups + run a restore drill; set up domain/TLS/CDN; verify email domain; stand up Alertmanager + uptime monitor; write/printed runbooks; brief organizer + scorers (one-scorer-per-match).

**Infrastructure tasks:** prod container deploy with restart policy + health gating; secret storage; CDN cache rules honoring `Cache-Control`; venue-NAT-aware rate-limit thresholds.

**Testing requirements:** restore drill (prove RPO/RTO); device matrix go/no-go; a load smoke (e.g., 50 concurrent spectators on public pages + 1 live scorer) to confirm cache + limits; an end-to-end dress rehearsal of a small bracket on the prod environment.

**Success criteria:** a restore from backup succeeds within RTO with ≤ RPO loss; verification/reset emails arrive in the inbox; the scorer passes every go/no-go device scenario; public pages stay up (CDN-served) through an API restart; alerts fire to a human on `/live` down and email backlog; every runbook is one page and rehearsed once. When all are met, **Operations → 🟢 and the pilot is a go.**

---

**Recommendation:** Run **PHI-1** before the first pilot. It adds no features; it converts a feature-complete product into an operable one. The single blocking dimension is **Operations** — backups + restore drill, a real production deploy with TLS, verified email, and runbooks — followed by scorer device validation. Everything else (alerting, scheduling, concurrent-scorer lease) is amber polish that can follow the first successful event.
