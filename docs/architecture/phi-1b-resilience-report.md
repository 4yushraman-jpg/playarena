# PHI-1B — Backup, Restore, Monitoring & Alerting — Implementation Report

**Status:** Implemented + validated (incl. a real restore drill). PROJECT_STATE not updated. Not committed.
**Mission:** take PlayArena from *deployable* to *recoverable and observable*. No new product/player features; GP-1/auth/RBAC untouched.

---

## 1. Architecture compliance
Per the "config over code" mandate, PHI-1B is overwhelmingly configuration, scripts, dashboards, alert rules, and runbooks. The **only** application code added is one isolated, justified observability scraper (tournament-day gauges via raw count queries — no domain coupling, no feature change). No auth/RBAC/GP-1 touched.

## 2. Files created
**Operational scripts** — `deploy/scripts/backup.sh`, `restore.sh`, `restore-drill.sh`.
**Alerting/observability** — `deploy/alertmanager/alertmanager.yml`; `backend/deploy/grafana/dashboards/pilot-day.json`.
**Runbooks** — `deploy/runbooks/`: `restore.md`, `backup.md`, `restore-drill-results.md`, `api-down.md`, `database-down.md`, `email-outage.md`, `public-outage.md`, `scorer-incident.md`, `rollback.md`, `uptime-monitoring.md`.
**This report.**

## 3. Files modified
- `backend/internal/platform/metrics/metrics.go` — 3 gauges (`playarena_tournaments_ongoing`, `playarena_matches_live`, `playarena_match_events_last_minute`).
- `backend/internal/bootstrap/observability.go` — `startTournamentActivityScraper` (30 s DB sample).
- `backend/internal/bootstrap/app.go` — start the scraper.
- `backend/deploy/prometheus/alerts.yaml` — `APIUnavailable` + a `playarena_pilot` group (BackupStale, DiskPressure, APIMemoryHigh, PublicPageErrors, LiveMatchesNotScoring).
- `backend/deploy/prometheus/prometheus.yml` — Alertmanager target + (commented) node job.
- `deploy/docker-compose.prod.yml` — `alertmanager` service (observability profile).

## 4. Backup architecture
**Two layers.** **Layer 1 — managed PITR** (provider WAL + daily snapshots): the primary path, minute-level RPO. **Layer 2 — nightly logical dump** (`backup.sh`: `pg_dump -Fc` → `pg_restore --list` verify → GPG encrypt → object-store upload → local rotation), portable and provider-independent, plus a success-timestamp metric for the `BackupStale` alert.
- **Retention:** local pruned after `BACKUP_RETENTION_DAYS` (14); remote via bucket lifecycle (~30 d).
- **Encryption:** `gpg --encrypt` at rest; key held only by the operator. **Storage:** approved object storage (Cloudflare R2 / S3) — dumps never linger unencrypted outside it. **Rotation:** local find-mtime + remote lifecycle.
- **RPO justification:** *Tournament day ≤ 5 min* — losing more than a few minutes of live scoring is unacceptable mid-event, so Layer-1 continuous WAL (RPO ≈ minutes) is mandatory while a tournament runs. *Normal ≤ 24 h* — for a grassroots platform between events, a nightly logical dump is sufficient and cheap; the client-side offline scoring queue independently protects in-flight events across an API restart (RPO ≈ 0 from the device).

## 5. Restore architecture
**RTO targets:** ≤ 15 min app-only, ≤ 60 min full DB restore. Path A **managed PITR** (preferred; rewind to seconds before an incident on a new instance, repoint `DATABASE_URL`). Path B **logical restore** (`restore.sh`, `pg_restore --clean --if-exists`, with a `prod`-overwrite guard). Plus full-environment-rebuild and app-only-recovery procedures. All in `deploy/runbooks/restore.md`, written to be executed by a new engineer with no tribal knowledge.

## 6. Restore drill results (MANDATORY — performed)
`deploy/scripts/restore-drill.sh` was executed: throwaway Postgres 17 → real migrations (000001–000030) → representative seed (org/2 teams/tournament/2 regs/completed match/2 events) → `pg_dump` → **DROP DATABASE** → `pg_restore` → verify.

| Metric | Result |
|---|---|
| Outcome | **PASS** |
| Data loss | **none** — all 6 table counts identical pre/post |
| Actual RTO (destroy→restored+verified) | **~3 s** (mechanism cost on a tiny dataset; prod RTO = provider restore latency, ≤ 60 min target) |
| Standings feed query post-restore | 1/1 ✓ |
| Public-page gate query post-restore | 1/1 ✓ |

Full record: `deploy/runbooks/restore-drill-results.md`. Re-run before every pilot.

## 7. Monitoring additions
Existing coverage was already strong (HTTP rps/latency/in-flight, DB pool, auth, email/webhook workers + dead-letters, outbox backlog, realtime). **Added:** the three tournament-activity gauges (live matches, ongoing tournaments, scoring rate) — the missing "is the tournament happening?" signal. **Pilot Day dashboard** (`pilot-day.json`): one screen — API up, live matches, ongoing tournaments, events/min, live spectators, backup age (stat tiles) over public traffic, 5xx+p99, DB pool, delivery backlog, rate-limit rejections, SSE drops. No dashboard hunting.

## 8. Alert rules (validated: `promtool check rules` → 18 rules OK)
| Alert | Sev | Threshold | Rationale / response |
|---|---|---|---|
| APIUnavailable | crit | `up==0` 1m | process gone → api-down.md (paired w/ external uptime) |
| DatabaseUnavailable | crit | empty pool OR 503 surge 1m | DB outage/exhaustion → database-down.md |
| HighErrorRate5xx | crit | >5% 5xx 2m | broad failure → triage logs / rollback |
| Email/WebhookWorkerStuck | crit | no ticks 10m | delivery halted → email-outage.md |
| Email/WebhookDeadLetterBurst | crit | >50 | provider rejecting → email-outage.md |
| BackupStale | crit | >26h or absent | data at risk → backup.md |
| DiskPressure | crit | <10% free 5m | host fill → free space (needs node_exporter) |
| HighP99Latency | warn | >2s 5m | slowness → investigate route |
| DBPoolNearSaturation | warn | >85% 5m | exhaustion risk |
| APIMemoryHigh | warn | RSS>700MB 10m | leak/limit (process collector, no node_exporter) |
| PublicPageErrors | warn | >0.2 public-5xx/s 3m | spectators failing → public-outage.md |
| LiveMatchesNotScoring | warn | live>0 & events/min==0 10m | scorer disconnected → scorer-incident.md |
| OutboxBacklogHigh / AuthAnomaly / TokenReplay / RealtimeDropped | warn | (pre-existing) | retained |

## 9. Alert delivery & external uptime
**Delivery:** Alertmanager → **Slack incoming webhook** (`alertmanager.yml`), critical routed immediately (10 s wait, 1 h repeat), warnings grouped (4 h). *Chosen because* it's free, instant, and needs no on-call rotation or PagerDuty cost for a one-weekend pilot; email is wired as a secondary path. **External uptime:** **UptimeRobot** (free, 1-min, independent of the host so it catches total outages Prometheus can't) on the API, frontend, and a public tournament page (keyword check) — setup in `uptime-monitoring.md`.

## 10. Validation results
- `go build ./...` ✓ · `go vet ./...` ✓ · `go test ./internal/platform/metrics/...` ✓ · gofmt clean.
- `promtool check rules alerts.yaml` → **SUCCESS, 18 rules** ✓ · `promtool check config prometheus.yml` → **valid, 1 rule file/18 rules** ✓ · `amtool check-config` → **valid (2 receivers)** ✓ · `pilot-day.json` → valid JSON ✓.
- `bash -n` on all three scripts ✓ · **restore drill executed → PASS** ✓.
- Frontend untouched this phase: `tsc --noEmit` ✓ (no regression); FE-8/PRI-1 suites unaffected.

## 11. Security findings → remediations
- **P0 — backups could expose PII (plaintext / wrong location).** Resolved: `backup.sh` GPG-encrypts at rest and uploads only to approved object storage; local staging is pruned; the dump verify never prints data.
- **P1 — accidental restore over production.** Resolved: `restore.sh` refuses a `*prod*` target unless `ALLOW_PROD_RESTORE=1`.
- **P1 — secret leakage in alert config / dashboards / logs.** Resolved: `alertmanager.yml` ships a `__REPLACE_ME__` placeholder (no real webhook) and `deploy/.gitignore` blocks `.env*`/secrets; the real value is mounted as a secret. Dashboards query metrics only (no secrets). App logs error *messages*, never credentials; config validation reports which var is wrong, not its value.
- **P1 — observability/Grafana exposure.** Resolved (carried from PHI-1A): the `:9090` port is never published; Prometheus/Grafana/Alertmanager stay on the private network (SSH-tunnel access); Grafana admin password is required (no default).
All **P0/P1 resolved.**

## 12. Remaining P2 findings
- DiskPressure/BackupStale require **node_exporter** (textfile collector) — documented; without it those two alerts simply don't fire (the nightly cron MAILTO still surfaces backup failure).
- Alert delivery is a **single Slack channel** (no escalation policy) — acceptable for a one-weekend pilot; upgrade to Better Stack/PagerDuty later.
- Secrets live in **env files** on the host — standard for a pilot; gitignored + recommend a secret store.
- `APIMemoryHigh` threshold is a fixed 700 MB proxy — tune to the container limit.

## 13. Pilot-day ops review & updated readiness score

**Outage simulation (detection → recovery):**
| Injected | Detection | Recovery | Data loss |
|---|---|---|---|
| API outage | `APIUnavailable` ≤1m + UptimeRobot ~1–2m | restart (api-down.md) ~2–3m | none (offline scoring queue reconciles) |
| Database outage | `DatabaseUnavailable` ≤1m | restart or PITR (restore.md), RTO ≤60m | ≤ RPO (minutes via PITR) — **restore proven** |
| Email outage | `EmailWorkerStuck` 5–10m | fix provider / manual links (email-outage.md) | none (queued, drains on recovery) |
| Network interruption | uptime monitor / dashboard | auto-reconnect; public pages from CDN | none (exactly-once queue) |

| Dimension | Was (PHI-1) | Now | Justification |
|---|---|---|---|
| Architecture | 🟢 | 🟢 | unchanged, sound |
| Reliability | 🟡 | 🟡 | DB recovery now proven; **concurrent-scorer lease + real-device validation still outstanding** (later slice) |
| Operations | 🔴 | 🟢 | backups (2 layers) + **tested** restore + runbooks + rollback |
| Security | 🟡 | 🟢 | backup encryption, restore guard, secret hygiene, private observability |
| Observability | 🟡 | 🟢 | alerts→Slack, external uptime, Pilot-Day dashboard, business metrics |
| Supportability | 🔴/🟡 | 🟢 | 10 executable runbooks covering every injected failure |

**Overall: 🟡 (was 🔴→🟡).** The blocking **Operations** dimension is now 🟢: PlayArena can lose a database and restore within target (proven), lose an API instance and detect+alert+recover via runbook. The remaining amber is **Reliability** — the concurrent-scorer lease and the real-device scorer validation — which is the next PHI-1 slice, not PHI-1B.

**Stopped at PHI-1B.** No PHI-1C / GP-2 / Reputation / Recruitment / FE-8D started. Nothing committed; the working tree contains only PHI-1B changes (plus prior uncommitted phase work in this session).
