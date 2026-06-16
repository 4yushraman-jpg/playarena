#!/usr/bin/env bash
# Pilot smoke — the tournament-day paths the happy-path test didn't cover:
#   A) Walkover / no-show in a knockout (FE-8A) + winner auto-advance
#   B) Group + knockout with qualifier resolution (FE-8C)
# All writes via the public API; match rows read via psql. Leaves data in place.
set -uo pipefail
BASE=http://localhost:8080/api/v1
PG=(docker exec -i playarena-postgres psql -U postgres -d playarena -tAq)

EMAIL="pilot@playarena.test"; PASS="Pilot123!"
j() { sed -n "s/.*\"$1\":\"\([^\"]*\)\".*/\1/p"; }
jb() { sed -n "s/.*\"$1\":\([a-z0-9]*\).*/\1/p"; }   # bool/number value
step() { printf '\n\033[1m== %s ==\033[0m\n' "$1"; }
sub() { printf '\033[36m-- %s --\033[0m\n' "$1"; }

declare -A IDX        # team_id -> creation index (for deterministic group results)

step "0. Pre-clean (row deletes only)"
"${PG[@]}" -c "DELETE FROM match_events; DELETE FROM player_tournament_stats; DELETE FROM team_tournament_stats; DELETE FROM matches; DELETE FROM tournament_registrations; DELETE FROM tournaments; DELETE FROM team_memberships; DELETE FROM teams; DELETE FROM players; DELETE FROM user_organization_roles; DELETE FROM organizations; DELETE FROM users;" >/dev/null && echo "  cleaned"

step "1-3. Organizer + org + 8 teams (each with an active member)"
REG=$(curl -s -X POST $BASE/auth/register -H 'Content-Type: application/json' -d "{\"email\":\"$EMAIL\",\"username\":\"pilot\",\"password\":\"$PASS\",\"full_name\":\"Pilot Organizer\"}")
VTOK=$(echo "$REG" | j verification_token | sed 's/=/%3D/g')
curl -s -o /dev/null "$BASE/auth/verify-email?token=$VTOK"
TOK=$(curl -s -X POST $BASE/auth/login -H 'Content-Type: application/json' -d "{\"email\":\"$EMAIL\",\"password\":\"$PASS\"}" | j access_token)
AUTH=(-H "Authorization: Bearer $TOK")
ORG=$(curl -s -X POST $BASE/organizations "${AUTH[@]}" -H 'Content-Type: application/json' -d '{"name":"Pilot Club","type":"club"}')
OSLUG=$(echo "$ORG" | j slug)
TOK=$(curl -s -X POST $BASE/auth/login -H 'Content-Type: application/json' -d "{\"email\":\"$EMAIL\",\"password\":\"$PASS\"}" | j access_token)
AUTH=(-H "Authorization: Bearer $TOK")
TEAMS=()
i=0
for n in Alpha Bravo Charlie Delta Echo Foxtrot Golf Hotel; do
  tid=$(curl -s -X POST "$BASE/organizations/$OSLUG/teams" "${AUTH[@]}" -H 'Content-Type: application/json' -d "{\"name\":\"$n\"}" | j id)
  pid=$(curl -s -X POST "$BASE/organizations/$OSLUG/players" "${AUTH[@]}" -H 'Content-Type: application/json' -d "{\"display_name\":\"$n Cap\"}" | j id)
  curl -s -o /dev/null -X POST "$BASE/organizations/$OSLUG/teams/$tid/members" "${AUTH[@]}" -H 'Content-Type: application/json' -d "{\"player_id\":\"$pid\"}"
  TEAMS+=("$tid"); IDX[$tid]=$i; i=$((i+1))
done
echo "  org=$OSLUG, ${#TEAMS[@]} teams ready"

# ── helpers ───────────────────────────────────────────────────────────────────
patch_t() { curl -s -o /dev/null -X PATCH "$BASE/organizations/$OSLUG/tournaments/$1" "${AUTH[@]}" -H 'Content-Type: application/json' -d "$2"; }
reg_approve() { # tid_tournament  team_id
  local rid=$(curl -s -X POST "$BASE/organizations/$OSLUG/tournaments/$1/registrations" "${AUTH[@]}" -H 'Content-Type: application/json' -d "{\"team_id\":\"$2\"}" | j id)
  curl -s -o /dev/null -X PATCH "$BASE/organizations/$OSLUG/tournaments/$1/registrations/$rid" "${AUTH[@]}" -H 'Content-Type: application/json' -d '{"status":"approved"}'
}
score_match() { # match_id  winner_team_id
  curl -s -o /dev/null -X PATCH "$BASE/organizations/$OSLUG/matches/$1" "${AUTH[@]}" -H 'Content-Type: application/json' -d '{"status":"live"}'
  for _ in 1 2 3; do curl -s -o /dev/null -X POST "$BASE/organizations/$OSLUG/matches/$1/events" "${AUTH[@]}" -H 'Content-Type: application/json' -d "{\"event_type\":\"raid_successful\",\"team_id\":\"$2\",\"payload\":{\"points\":2}}"; done
  curl -s -X PATCH "$BASE/organizations/$OSLUG/matches/$1" "${AUTH[@]}" -H 'Content-Type: application/json' -d "{\"status\":\"completed\",\"winner_team_id\":\"$2\"}"
}

#######################################################################
step "SCENARIO A — Walkover in a knockout (4 teams)"
#######################################################################
TA=$(curl -s -X POST "$BASE/organizations/$OSLUG/tournaments" "${AUTH[@]}" -H 'Content-Type: application/json' -d '{"name":"Walkover Cup","sport":"kabaddi","format":"knockout","participant_type":"team"}')
TAID=$(echo "$TA" | j id); TASLUG=$(echo "$TA" | j slug)
patch_t "$TAID" '{"visibility":"public"}'; patch_t "$TAID" '{"status":"registration_open"}'
for k in 0 1 2 3; do reg_approve "$TAID" "${TEAMS[$k]}"; done
patch_t "$TAID" '{"status":"registration_closed"}'
GEN=$(curl -s -X POST "$BASE/organizations/$OSLUG/tournaments/$TAID/fixtures/generate" "${AUTH[@]}" -H 'Content-Type: application/json' -d '{"seed_strategy":"registration"}')
echo "  generated knockout matches: $(echo "$GEN" | jb generated)"
patch_t "$TAID" '{"status":"ongoing"}'

sub "Semi 1 -> WALKOVER (home wins, no-show), Semi 2 -> normal"
ROWS=$("${PG[@]}" -c "SELECT id||','||home_team_id||','||away_team_id FROM matches WHERE tournament_id='$TAID' AND status='scheduled' AND home_team_id IS NOT NULL AND away_team_id IS NOT NULL ORDER BY round_number, match_number;")
n=0
while IFS=, read -r mid home away; do
  [ -z "$mid" ] && continue
  if [ $n -eq 0 ]; then
    WO=$(curl -s -X POST "$BASE/organizations/$OSLUG/matches/$mid/walkover" "${AUTH[@]}" -H 'Content-Type: application/json' -d '{"winner":"home","reason":"Away team failed to appear; 15-min grace expired"}')
    echo "  semi1 walkover: status=$(echo "$WO" | j status) is_walkover=$(echo "$WO" | jb is_walkover) winner_set=$([ -n "$(echo "$WO" | j winner_team_id)" ] && echo yes || echo no)"
  else
    R=$(score_match "$mid" "$home"); echo "  semi2 normal: status=$(echo "$R" | j status)"
  fi
  n=$((n+1))
done <<< "$ROWS"

sub "Final (one finalist arrived via walkover) -> score"
FROW=$("${PG[@]}" -c "SELECT id||','||home_team_id||','||away_team_id FROM matches WHERE tournament_id='$TAID' AND status='scheduled' AND home_team_id IS NOT NULL AND away_team_id IS NOT NULL ORDER BY round_number, match_number;")
IFS=, read -r fmid fhome faway <<< "$FROW"
[ -n "$fmid" ] && { R=$(score_match "$fmid" "$fhome"); echo "  final: status=$(echo "$R" | j status)"; } || echo "  !! final not populated (bracket advance FAILED)"

sub "Verify"
"${PG[@]}" -c "SELECT round_name, status, is_walkover, home_score AS h, away_score AS a FROM matches WHERE tournament_id='$TAID' ORDER BY round_number, match_number;" | sed 's/^/  /'
patch_t "$TAID" '{"status":"completed"}'
echo "  standings HTTP: $(curl -s -o /dev/null -w '%{http_code}' "$BASE/public/orgs/$OSLUG/tournaments/$TASLUG/standings")"

#######################################################################
step "SCENARIO B — Group + knockout, qualifier resolution (8 teams, 2 groups, top 2)"
#######################################################################
TB=$(curl -s -X POST "$BASE/organizations/$OSLUG/tournaments" "${AUTH[@]}" -H 'Content-Type: application/json' -d '{"name":"Group Cup","sport":"kabaddi","format":"group_knockout","participant_type":"team"}')
TBID=$(echo "$TB" | j id); TBSLUG=$(echo "$TB" | j slug)
patch_t "$TBID" '{"visibility":"public"}'; patch_t "$TBID" '{"status":"registration_open"}'
for k in 0 1 2 3 4 5 6 7; do reg_approve "$TBID" "${TEAMS[$k]}"; done
patch_t "$TBID" '{"status":"registration_closed"}'
GENB=$(curl -s -X POST "$BASE/organizations/$OSLUG/tournaments/$TBID/fixtures/generate" "${AUTH[@]}" -H 'Content-Type: application/json' -d '{"seed_strategy":"registration","groups":2,"qualifiers_per_group":2}')
echo "  generated total matches: $(echo "$GENB" | jb generated)"
patch_t "$TBID" '{"status":"ongoing"}'

sub "Score ALL group-stage matches (lower-index team wins -> deterministic standings)"
GROWS=$("${PG[@]}" -c "SELECT id||','||home_team_id||','||away_team_id FROM matches WHERE tournament_id='$TBID' AND group_label IS NOT NULL AND status='scheduled' ORDER BY group_label, match_number;")
gc=0
while IFS=, read -r mid home away; do
  [ -z "$mid" ] && continue
  if [ "${IDX[$home]}" -lt "${IDX[$away]}" ]; then win=$home; else win=$away; fi
  score_match "$mid" "$win" >/dev/null
  gc=$((gc+1))
done <<< "$GROWS"
echo "  scored $gc group matches"

sub "Group standings (each group, ranked)"
"${PG[@]}" -c "SELECT t.name, s.matches_won AS won, s.points FROM team_tournament_stats s JOIN teams t ON t.id=s.team_id WHERE s.tournament_id='$TBID' ORDER BY s.matches_won DESC, s.points DESC;" 2>/dev/null | sed 's/^/  /' || echo "  (stats query skipped)"

sub "Resolve qualifiers -> fill the knockout bracket"
RQ=$(curl -s -o /dev/null -w "%{http_code}" -X POST "$BASE/organizations/$OSLUG/tournaments/$TBID/fixtures/resolve-qualifiers" "${AUTH[@]}" -H 'Content-Type: application/json' -d '{}')
echo "  resolve-qualifiers HTTP: $RQ"

sub "Score the knockout rounds (winners auto-advance)"
for round in 1 2 3; do
  KROWS=$("${PG[@]}" -c "SELECT id||','||home_team_id||','||away_team_id FROM matches WHERE tournament_id='$TBID' AND group_label IS NULL AND status='scheduled' AND home_team_id IS NOT NULL AND away_team_id IS NOT NULL ORDER BY round_number, match_number;")
  [ -z "$KROWS" ] && { echo "  round $round: none playable"; break; }
  while IFS=, read -r mid home away; do
    [ -z "$mid" ] && continue
    if [ "${IDX[$home]}" -lt "${IDX[$away]}" ]; then win=$home; else win=$away; fi
    R=$(score_match "$mid" "$win"); echo "  ko match $mid -> ${R:+done}"
  done <<< "$KROWS"
done

sub "Verify"
"${PG[@]}" -c "SELECT COALESCE(group_label,'KO') AS stage, round_name, count(*) AS matches, count(*) FILTER (WHERE status='completed') AS completed FROM matches WHERE tournament_id='$TBID' GROUP BY 1,2 ORDER BY 1,2;" | sed 's/^/  /'
patch_t "$TBID" '{"status":"completed"}'
CH=$("${PG[@]}" -c "SELECT t.name FROM matches m JOIN teams t ON t.id=m.winner_team_id WHERE m.tournament_id='$TBID' AND m.group_label IS NULL ORDER BY m.round_number DESC, m.match_number DESC LIMIT 1;")
echo "  champion: ${CH:-<none>}"
echo "  public pages: tournament=$(curl -s -o /dev/null -w '%{http_code}' "$BASE/public/orgs/$OSLUG/tournaments/$TBSLUG") standings=$(curl -s -o /dev/null -w '%{http_code}' "$BASE/public/orgs/$OSLUG/tournaments/$TBSLUG/standings")"

step "RESULT — both scenarios done"
echo "  Scenario A (walkover):       org=$OSLUG tournament=$TASLUG"
echo "  Scenario B (group+knockout): org=$OSLUG tournament=$TBSLUG"
