#!/usr/bin/env bash
# PlayArena — nightly logical backup (PHI-1B).
#
# Layer 2 of the backup strategy: the managed Postgres provider's PITR/snapshots
# are layer 1 (RPO ~minutes). This script is a portable, provider-independent
# logical dump (pg_dump custom format) — encrypted at rest, verified, rotated.
#
# Usage:  DATABASE_URL=... ./backup.sh
# Env:
#   DATABASE_URL        (required) postgres connection string (sslmode=require)
#   BACKUP_DIR          local staging dir              (default ./backups)
#   BACKUP_RETENTION_DAYS  delete local dumps older than this (default 14)
#   BACKUP_GPG_RECIPIENT   if set, encrypt with `gpg --encrypt -r <recipient>`
#   BACKUP_UPLOAD_CMD      if set, run `<cmd> <file>` to push to object storage
#                          (e.g. "aws s3 cp - s3://bucket/" or an rclone wrapper)
#
# Exit non-zero on ANY failure so the scheduler (cron/systemd timer) alerts.
set -euo pipefail

: "${DATABASE_URL:?DATABASE_URL is required}"
BACKUP_DIR="${BACKUP_DIR:-./backups}"
RETENTION="${BACKUP_RETENTION_DAYS:-14}"
ts="$(date -u +%Y%m%dT%H%M%SZ)"
mkdir -p "$BACKUP_DIR"
file="$BACKUP_DIR/playarena-$ts.dump"

echo "[backup] $ts → $file"

# Custom format (-Fc): compressed, parallel-restorable, selective. No PII in the
# filename. The dump itself contains data and MUST live only in approved storage.
pg_dump --dbname="$DATABASE_URL" --format=custom --no-owner --no-privileges --file="$file"

# Verify the dump is structurally valid before we trust it (an unverified backup
# does not count). --list parses the archive TOC without touching a database.
pg_restore --list "$file" >/dev/null
echo "[backup] verified archive TOC"

out="$file"
if [[ -n "${BACKUP_GPG_RECIPIENT:-}" ]]; then
  gpg --yes --batch --encrypt --recipient "$BACKUP_GPG_RECIPIENT" --output "$file.gpg" "$file"
  rm -f "$file"
  out="$file.gpg"
  echo "[backup] encrypted → $out"
fi

if [[ -n "${BACKUP_UPLOAD_CMD:-}" ]]; then
  # shellcheck disable=SC2086
  $BACKUP_UPLOAD_CMD "$out"
  echo "[backup] uploaded via BACKUP_UPLOAD_CMD"
fi

# Rotation: prune local dumps older than retention (remote rotation is the
# object-store lifecycle policy's job).
find "$BACKUP_DIR" -name 'playarena-*.dump*' -type f -mtime "+$RETENTION" -print -delete || true

# Emit a success timestamp for the BackupStale alert (node_exporter textfile
# collector picks up *.prom files from BACKUP_TEXTFILE_DIR). Only on success.
if [[ -n "${BACKUP_TEXTFILE_DIR:-}" ]]; then
  tmp="$(mktemp)"
  {
    echo "# HELP playarena_backup_last_success_timestamp_seconds Unix time of the last successful logical backup."
    echo "# TYPE playarena_backup_last_success_timestamp_seconds gauge"
    echo "playarena_backup_last_success_timestamp_seconds $(date +%s)"
  } > "$tmp"
  mv "$tmp" "$BACKUP_TEXTFILE_DIR/playarena_backup.prom"
fi

echo "[backup] OK ($out)"
