# Runbook — Backups

Two layers. **Both must be on before the pilot.**

## Layer 1 — Managed PITR (primary, RPO ≈ minutes)
The managed Postgres provider's continuous WAL archiving + daily snapshots.
- Verify in the provider console that **PITR is enabled** and note the retention
  window (e.g. 7 days). This is what gives the tournament-day RPO ≤ 5 min target.
- Nothing to run; it is the provider's job. **Confirm it is on — do not assume.**

## Layer 2 — Nightly logical dump (portable, provider-independent)
`deploy/scripts/backup.sh` → `pg_dump -Fc`, verified (`pg_restore --list`),
encrypted, uploaded, rotated.

**Schedule (cron on the host):**
```cron
# 02:30 UTC nightly; alert on failure via MAILTO.
MAILTO=ops@playarena.app
30 2 * * *  DATABASE_URL='postgres://…?sslmode=require' \
            BACKUP_DIR=/var/backups/playarena \
            BACKUP_RETENTION_DAYS=14 \
            BACKUP_GPG_RECIPIENT=ops@playarena.app \
            BACKUP_TEXTFILE_DIR=/var/lib/node_exporter/textfile \
            BACKUP_UPLOAD_CMD='rclone copyto - r2:playarena-backups/' \
            /opt/playarena/deploy/scripts/backup.sh >> /var/log/playarena-backup.log 2>&1
```
- **Encryption:** set `BACKUP_GPG_RECIPIENT` so dumps are encrypted at rest
  (`gpg --encrypt`). The recipient's private key is held only by the operator.
- **Storage:** `BACKUP_UPLOAD_CMD` pushes to object storage (Cloudflare R2 / S3).
  Set a bucket **lifecycle/retention** policy for remote rotation (e.g. 30 days).
- **Freshness alert:** `BACKUP_TEXTFILE_DIR` makes the script write
  `playarena_backup_last_success_timestamp_seconds`; node_exporter's textfile
  collector exposes it and the **BackupStale** alert fires if >26 h old.
- **Retention:** local dumps pruned after `BACKUP_RETENTION_DAYS`; remote by the
  bucket policy.

## Verification (run weekly + once before the pilot)
- A fresh dump file appears and is non-trivial in size; `pg_restore --list` of it
  succeeds (the script does this automatically).
- **Run the restore drill** (`deploy/scripts/restore-drill.sh`) — an untested
  backup does not count. Results: `restore-drill-results.md`.

## Symptoms it's broken
- `BackupStale` alert; no new files in the bucket; cron MAILTO error mail.
## Immediate action
- Run `backup.sh` manually and read the log; fix the failing step (creds, disk,
  network); confirm a new success timestamp.
