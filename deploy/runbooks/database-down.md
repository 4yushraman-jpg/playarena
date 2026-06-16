# Runbook — Database Unavailable

**Symptoms:** `DatabaseUnavailable` alert (empty pool or 503 surge); `DBPoolNearSaturation`;
API logs full of connection errors; writes/reads failing.

**Diagnosis**
```sh
# Managed provider console: instance status, CPU, connections, incidents.
psql "$DATABASE_URL" -c "SELECT 1;"                          # can we connect at all?
# On the Pilot Day dashboard: DB Pool (acquired/max). Saturated vs zero?
```
- **Saturated** (acquired ≈ max): connection exhaustion (a query storm / leak),
  not an outage. **Zero / cannot connect:** provider outage or network/credentials.

**Immediate actions**
- Saturation: identify and stop the offending client; restart `api` to reset the
  pool; raise the provider's connection limit if genuinely undersized.
- Provider outage: check the provider status page; if it's a transient blip the
  app auto-reconnects (pgx pool) — wait and watch.
- Credentials/endpoint changed: fix `DATABASE_URL`, recreate `api`.

**Recovery actions**
- Sustained outage → restore to a new instance per **restore.md** (PITR preferred)
  and repoint `DATABASE_URL`.

**Escalation**
- Provider incident with no ETA, or data corruption suspected → platform owner;
  begin PITR restore in parallel to a new instance.

**Verification**
- `psql … SELECT 1` ok; pool acquired drops to baseline; `DatabaseUnavailable`
  resolves; a write (e.g. score an event) and a read (standings) both succeed.
