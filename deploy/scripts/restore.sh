#!/usr/bin/env bash
# PlayArena — restore from a logical backup (PHI-1B).
#
# Restores a pg_dump custom-format archive into a target database. Used for the
# layer-2 restore path and the restore drill. For routine recovery prefer the
# managed provider's PITR (smaller RPO); use this when you need a portable dump
# or are rebuilding on a fresh provider.
#
# Usage:  TARGET_DATABASE_URL=... ./restore.sh <archive.dump[.gpg]>
# Env:
#   TARGET_DATABASE_URL  (required) where to restore (MUST be empty/disposable)
#   RESTORE_JOBS         parallel restore workers (default 4)
#
# Safety: refuses to run without an explicit target; never restores into a URL
# named "prod" unless ALLOW_PROD_RESTORE=1 (guards against fat-finger overwrite).
set -euo pipefail

archive="${1:?usage: restore.sh <archive.dump[.gpg]>}"
: "${TARGET_DATABASE_URL:?TARGET_DATABASE_URL is required}"
JOBS="${RESTORE_JOBS:-4}"

if [[ "$TARGET_DATABASE_URL" == *prod* && "${ALLOW_PROD_RESTORE:-0}" != "1" ]]; then
  echo "[restore] refusing: target looks like prod (set ALLOW_PROD_RESTORE=1 to override)" >&2
  exit 2
fi

work="$archive"
if [[ "$archive" == *.gpg ]]; then
  work="${archive%.gpg}"
  gpg --yes --batch --decrypt --output "$work" "$archive"
  echo "[restore] decrypted → $work"
fi

echo "[restore] restoring $work → $TARGET_DATABASE_URL"
# --clean --if-exists makes the restore idempotent onto a possibly-dirty target.
pg_restore --dbname="$TARGET_DATABASE_URL" --no-owner --no-privileges \
  --clean --if-exists --jobs="$JOBS" "$work"

echo "[restore] OK"
