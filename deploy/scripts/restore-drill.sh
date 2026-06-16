#!/usr/bin/env bash
# PlayArena — restore drill (PHI-1B, MANDATORY).
#
# Proves the backup→destroy→restore round-trip on a disposable Postgres:
#   1. spin a throwaway Postgres 17 container
#   2. apply the real migrations (000001..)
#   3. seed representative data (org/teams/tournament/regs/match/events)
#   4. logical backup (pg_dump -Fc)
#   5. DESTROY the database (DROP ... WITH FORCE)
#   6. restore from the backup (pg_restore)
#   7. verify row counts match and the standings + public-page queries still work
#   8. measure actual RTO and data loss
#
# An untested restore does not count. Run from anywhere:  bash restore-drill.sh
set -euo pipefail

# Git Bash (MSYS) rewrites container-absolute paths like /tmp/... into Windows
# paths when passed to docker. Disable that translation; harmless on real Linux.
export MSYS_NO_PATHCONV=1

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
MIG="$ROOT/backend/db/migrations"
CT=padrill-db
PW=drilltest
PSQL=(docker exec -i -e PGPASSWORD=$PW "$CT" psql -v ON_ERROR_STOP=1 -U postgres -d playarena -t -A)

cleanup() { docker rm -f "$CT" >/dev/null 2>&1 || true; }
trap cleanup EXIT

echo "[drill] starting throwaway Postgres 17…"
cleanup
docker run -d --name "$CT" -e POSTGRES_PASSWORD=$PW -e POSTGRES_DB=playarena postgres:17 >/dev/null
until docker exec "$CT" pg_isready -U postgres -d playarena >/dev/null 2>&1; do sleep 1; done

echo "[drill] applying migrations…"
for f in "$MIG"/*.up.sql; do
  docker exec -i -e PGPASSWORD=$PW "$CT" psql -q -v ON_ERROR_STOP=1 -U postgres -d playarena < "$f"
done

echo "[drill] seeding representative data…"
docker exec -i -e PGPASSWORD=$PW "$CT" psql -q -v ON_ERROR_STOP=1 -U postgres -d playarena <<'SQL'
INSERT INTO organizations (id,name,slug,type,status) VALUES
  ('11111111-1111-1111-1111-111111111111','Drill Org','drill-org','club','active');
INSERT INTO teams (id,organization_id,name,slug,status) VALUES
  ('22222222-2222-2222-2222-222222222222','11111111-1111-1111-1111-111111111111','Team Alpha','team-alpha','active'),
  ('33333333-3333-3333-3333-333333333333','11111111-1111-1111-1111-111111111111','Team Beta','team-beta','active');
INSERT INTO tournaments (id,organization_id,name,slug,sport,format,participant_type,status,currency,visibility) VALUES
  ('44444444-4444-4444-4444-444444444444','11111111-1111-1111-1111-111111111111','Winter Cup','winter-cup','kabaddi','knockout','team','ongoing','INR','public');
INSERT INTO tournament_registrations (id,tournament_id,organization_id,team_id,status,registered_at) VALUES
  ('aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa','44444444-4444-4444-4444-444444444444','11111111-1111-1111-1111-111111111111','22222222-2222-2222-2222-222222222222','approved',NOW()),
  ('bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb','44444444-4444-4444-4444-444444444444','11111111-1111-1111-1111-111111111111','33333333-3333-3333-3333-333333333333','approved',NOW());
INSERT INTO matches (id,tournament_id,organization_id,home_team_id,away_team_id,status,winner_team_id,home_score,away_score,started_at,ended_at) VALUES
  ('55555555-5555-5555-5555-555555555555','44444444-4444-4444-4444-444444444444','11111111-1111-1111-1111-111111111111','22222222-2222-2222-2222-222222222222','33333333-3333-3333-3333-333333333333','completed','22222222-2222-2222-2222-222222222222',30,20,NOW()-INTERVAL '1 hour',NOW());
INSERT INTO match_events (match_id,organization_id,sequence_number,event_type,team_id,recorded_at) VALUES
  ('55555555-5555-5555-5555-555555555555','11111111-1111-1111-1111-111111111111',1,'raid_successful','22222222-2222-2222-2222-222222222222',NOW()),
  ('55555555-5555-5555-5555-555555555555','11111111-1111-1111-1111-111111111111',2,'tackle_successful','33333333-3333-3333-3333-333333333333',NOW());
SQL

TABLES="organizations teams tournaments tournament_registrations matches match_events"
counts() { for t in $TABLES; do printf '%s=%s\n' "$t" "$("${PSQL[@]}" -c "SELECT COUNT(*) FROM $t")"; done; }

pre="$(counts)"
echo "[drill] pre-backup counts:"; echo "$pre" | sed 's/^/  /'

echo "[drill] backing up…"
docker exec -e PGPASSWORD=$PW "$CT" pg_dump -U postgres -Fc -d playarena -f /tmp/drill.dump

echo "[drill] DESTROYING database…"
t_start=$(date +%s)
docker exec -i -e PGPASSWORD=$PW "$CT" psql -q -v ON_ERROR_STOP=1 -U postgres -d postgres \
  -c "DROP DATABASE playarena WITH (FORCE);" -c "CREATE DATABASE playarena;"

echo "[drill] restoring…"
docker exec -e PGPASSWORD=$PW "$CT" pg_restore -U postgres --no-owner --no-privileges -d playarena /tmp/drill.dump
t_end=$(date +%s)

post="$(counts)"
echo "[drill] post-restore counts:"; echo "$post" | sed 's/^/  /'

# ── verification ────────────────────────────────────────────────────────────
fail=0
[[ "$pre" == "$post" ]] || { echo "[drill] FAIL: row counts differ"; fail=1; }

standings="$("${PSQL[@]}" -c "SELECT COUNT(*) FROM matches WHERE tournament_id='44444444-4444-4444-4444-444444444444' AND status IN ('completed','walkover')")"
[[ "$standings" == "1" ]] || { echo "[drill] FAIL: standings feed query ($standings != 1)"; fail=1; }

publicq="$("${PSQL[@]}" -c "SELECT COUNT(*) FROM tournaments t JOIN organizations o ON o.id=t.organization_id WHERE o.slug='drill-org' AND t.slug='winter-cup' AND t.status<>'draft' AND t.visibility IN ('unlisted','public') AND o.status='active'")"
[[ "$publicq" == "1" ]] || { echo "[drill] FAIL: public page query ($publicq != 1)"; fail=1; }

rto=$((t_end - t_start))
echo "────────────────────────────────────────────────"
echo "[drill] RTO (destroy→restored): ${rto}s"
echo "[drill] data loss: $([[ "$pre" == "$post" ]] && echo 'none (counts identical)' || echo 'DATA LOST')"
echo "[drill] standings query: $standings/1 · public query: $publicq/1"
if [[ $fail -eq 0 ]]; then echo "[drill] RESULT: PASS"; else echo "[drill] RESULT: FAIL"; exit 1; fi
