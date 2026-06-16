# Runbook — Database Restore

Executable by a new engineer with no tribal knowledge. Two recovery paths:
**(A) managed PITR** (preferred, smallest RPO) and **(B) logical-dump restore**
(portable / provider rebuild). Targets: **RTO ≤ 60 min**, **RPO ≤ 5 min**
(tournament day) / **≤ 24 h** (normal).

## When to use which
- **Accidental data change / recent corruption, same provider** → **PITR** (A):
  rewind to a timestamp seconds before the incident. Smallest data loss.
- **Provider lost / rebuilding elsewhere / verifying a dump** → **logical restore** (B).

---

## A. Managed PITR restore (preferred)
1. Open the managed-Postgres console (Neon / Supabase / RDS).
2. Choose **Point-in-Time Restore** → pick the timestamp just **before** the
   incident (the console shows the available window; WAL gives minute precision).
3. Restore into a **new** instance/branch (never overwrite the live one blind).
4. Get the new connection string; smoke-test it (counts, latest match):
   ```sh
   psql "$NEW_URL" -c "SELECT count(*) FROM tournaments; SELECT max(updated_at) FROM matches;"
   ```
5. Repoint the app: set `DATABASE_URL=$NEW_URL` in `backend/.env.production`,
   then `docker compose -f deploy/docker-compose.prod.yml up -d api` (recreates
   the container with the new URL). Verify via the API runbook's checks.

## B. Logical-dump restore
Prereq: a dump from `deploy/scripts/backup.sh` (`.dump` or `.dump.gpg`) and a
**target** database (a fresh managed instance or a clean DB).
```sh
export TARGET_DATABASE_URL='postgres://USER:PASS@HOST:5432/playarena?sslmode=require'
bash deploy/scripts/restore.sh /path/to/playarena-<ts>.dump        # or .dump.gpg
```
The script decrypts (if `.gpg`), runs `pg_restore --clean --if-exists`, and is
idempotent onto a dirty target. Then repoint the app as in A.5.

---

## Full environment rebuild (host lost)
1. New VM with Docker + Compose; `git` the repo (or pull images).
2. Restore the DB (A or B) → get `DATABASE_URL`.
3. `cp backend/.env.production.example backend/.env.production` and fill secrets
   from the secret store; set `DATABASE_URL`, `FRONTEND_URL`, email creds.
4. Apply any missing migrations: `docker compose -f deploy/docker-compose.prod.yml run --rm migrate`.
5. Bring up: `NEXT_PUBLIC_API_URL=https://api.<domain> docker compose -f deploy/docker-compose.prod.yml up -d --build`.
6. Re-point Cloudflare DNS to the new origin IP (and re-mount the origin cert).

## Application recovery (no DB loss — API/web only)
`docker compose -f deploy/docker-compose.prod.yml up -d api web` — the scorer
offline queues reconcile on reconnect (exactly-once), so in-flight scoring is not
lost across an API restart.

## Verification (after any restore)
- `psql "$DATABASE_URL" -c "SELECT count(*) FROM organizations, ..."` matches the
  expected magnitude; `SELECT max(updated_at) FROM matches` ≈ incident time.
- API `/live` (internal) green; a known tournament's public page loads logged-out;
  standings render; a completed match still shows its result.

## Escalation
If PITR window doesn't cover the incident, or both PITR and the latest dump are
unusable → escalate to the platform owner; fall back to the **previous** nightly
dump (accepting up to 24 h loss) and announce the data-loss window to the organizer.
