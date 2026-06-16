#!/usr/bin/env bash
# Organizer happy-path smoke test (pilot readiness gate #3).
# Drives the FULL flow via the public API: register -> verify -> create org ->
# teams -> tournament -> registrations -> approve -> generate fixtures -> score
# matches live -> complete -> bracket auto-advance -> complete tournament.
# Reads match IDs via psql (read-only). Leaves all data in place.
set -uo pipefail
BASE=http://localhost:8080/api/v1
PSQL=(docker exec -i playarena-postgres psql -U postgres -d playarena -tAq)

EMAIL="organizer@playarena.test"
PASS="Organizer123!"
UNAME="organizer"

j() { sed -n "s/.*\"$1\":\"\([^\"]*\)\".*/\1/p"; }   # extract first "key":"value"
step() { printf '\n\033[1m== %s ==\033[0m\n' "$1"; }

step "0. Pre-clean (row deletes only — reference tables untouched)"
docker exec -i playarena-postgres psql -U postgres -d playarena -q -c "
DELETE FROM match_events; DELETE FROM matches; DELETE FROM tournament_registrations;
DELETE FROM tournaments; DELETE FROM team_memberships; DELETE FROM teams;
DELETE FROM players; DELETE FROM user_organization_roles; DELETE FROM organizations;
DELETE FROM users;" && echo "  cleaned"

step "1. Register organizer ($EMAIL)"
REG=$(curl -s -X POST $BASE/auth/register -H 'Content-Type: application/json' \
  -d "{\"email\":\"$EMAIL\",\"username\":\"$UNAME\",\"password\":\"$PASS\",\"full_name\":\"Demo Organizer\"}")
echo "$REG"
VTOK=$(echo "$REG" | j verification_token | sed 's/=/%3D/g')

step "2. Verify email"
curl -s -o /dev/null -w "  verify status=%{http_code}\n" "$BASE/auth/verify-email?token=$VTOK"

step "3. Login (onboarding token expected)"
LOGIN=$(curl -s -X POST $BASE/auth/login -H 'Content-Type: application/json' -d "{\"email\":\"$EMAIL\",\"password\":\"$PASS\"}")
TOK=$(echo "$LOGIN" | j access_token)
echo "  scope=$(echo "$LOGIN" | j scope)"
AUTH=(-H "Authorization: Bearer $TOK")

step "4. Create organization 'Demo Kabaddi Club'"
ORG=$(curl -s -X POST $BASE/organizations "${AUTH[@]}" -H 'Content-Type: application/json' \
  -d '{"name":"Demo Kabaddi Club","type":"club","description":"Pilot smoke-test club"}')
echo "$ORG"
OSLUG=$(echo "$ORG" | j slug)

step "5. Re-login -> organizer token (1 org auto-selected)"
LOGIN2=$(curl -s -X POST $BASE/auth/login -H 'Content-Type: application/json' -d "{\"email\":\"$EMAIL\",\"password\":\"$PASS\"}")
TOK=$(echo "$LOGIN2" | j access_token)
AUTH=(-H "Authorization: Bearer $TOK")
echo "  scope=$(echo "$LOGIN2" | j scope)  org=$OSLUG"

step "6. Create 4 teams, each with a player + active membership"
TEAM_IDS=()
for n in Raiders Titans Panthers Warriors; do
  R=$(curl -s -X POST "$BASE/organizations/$OSLUG/teams" "${AUTH[@]}" -H 'Content-Type: application/json' -d "{\"name\":\"$n\"}")
  ID=$(echo "$R" | j id); TEAM_IDS+=("$ID")
  # A team must have >=1 active member before it can register.
  P=$(curl -s -X POST "$BASE/organizations/$OSLUG/players" "${AUTH[@]}" -H 'Content-Type: application/json' -d "{\"display_name\":\"$n Captain\"}")
  PID=$(echo "$P" | j id)
  M=$(curl -s -o /dev/null -w "%{http_code}" -X POST "$BASE/organizations/$OSLUG/teams/$ID/members" "${AUTH[@]}" -H 'Content-Type: application/json' -d "{\"player_id\":\"$PID\"}")
  echo "  $n -> team=$ID player=$PID member_add=$M"
done

step "7. Create tournament 'Summer Kabaddi Cup' (knockout)"
TRMT=$(curl -s -X POST "$BASE/organizations/$OSLUG/tournaments" "${AUTH[@]}" -H 'Content-Type: application/json' \
  -d '{"name":"Summer Kabaddi Cup","sport":"kabaddi","format":"knockout","participant_type":"team"}')
echo "$TRMT"
TID=$(echo "$TRMT" | j id); TSLUG=$(echo "$TRMT" | j slug)

step "7b. Set visibility=public (PATCH)"
VIS=$(curl -s -X PATCH "$BASE/organizations/$OSLUG/tournaments/$TID" "${AUTH[@]}" -H 'Content-Type: application/json' -d '{"visibility":"public"}')
echo "  visibility=$(echo "$VIS" | j visibility)"

step "8. draft -> registration_open"
curl -s -o /dev/null -w "  status=%{http_code}\n" -X PATCH "$BASE/organizations/$OSLUG/tournaments/$TID" "${AUTH[@]}" -H 'Content-Type: application/json' -d '{"status":"registration_open"}'

step "9. Register + approve all 4 teams"
for tid in "${TEAM_IDS[@]}"; do
  RR=$(curl -s -w "\nHTTP:%{http_code}" -X POST "$BASE/organizations/$OSLUG/tournaments/$TID/registrations" "${AUTH[@]}" -H 'Content-Type: application/json' -d "{\"team_id\":\"$tid\"}")
  CODE=$(echo "$RR" | sed -n 's/.*HTTP:\([0-9]*\)$/\1/p')
  RID=$(echo "$RR" | j id)
  AP=$(curl -s -o /dev/null -w "%{http_code}" -X PATCH "$BASE/organizations/$OSLUG/tournaments/$TID/registrations/$RID" "${AUTH[@]}" -H 'Content-Type: application/json' -d '{"status":"approved"}')
  echo "  team $tid -> POST=$CODE reg=$RID approve=$AP"
  [ "$CODE" != "201" ] && echo "    body: $(echo "$RR" | sed 's/HTTP:[0-9]*$//')"
done

step "10. registration_open -> registration_closed"
curl -s -o /dev/null -w "  status=%{http_code}\n" -X PATCH "$BASE/organizations/$OSLUG/tournaments/$TID" "${AUTH[@]}" -H 'Content-Type: application/json' -d '{"status":"registration_closed"}'

step "11. Generate fixtures (knockout bracket)"
GEN=$(curl -s -X POST "$BASE/organizations/$OSLUG/tournaments/$TID/fixtures/generate" "${AUTH[@]}" -H 'Content-Type: application/json' -d '{"seed_strategy":"registration"}')
echo "  generated=$(echo "$GEN" | sed -n 's/.*"generated":\([0-9]*\).*/\1/p')"

step "12. registration_closed -> ongoing"
curl -s -o /dev/null -w "  status=%{http_code}\n" -X PATCH "$BASE/organizations/$OSLUG/tournaments/$TID" "${AUTH[@]}" -H 'Content-Type: application/json' -d '{"status":"ongoing"}'

# Score every match that has both participants set, in round order; winners
# auto-advance (FE-8B) so the final fills in after the semis, then gets scored.
score_match() {
  local mid=$1 home=$2
  curl -s -o /dev/null -X PATCH "$BASE/organizations/$OSLUG/matches/$mid" "${AUTH[@]}" -H 'Content-Type: application/json' -d '{"status":"live"}'
  for i in 1 2 3; do
    curl -s -o /dev/null -X POST "$BASE/organizations/$OSLUG/matches/$mid/events" "${AUTH[@]}" -H 'Content-Type: application/json' -d "{\"event_type\":\"raid_successful\",\"team_id\":\"$home\",\"payload\":{\"points\":2}}"
  done
  local DONE=$(curl -s -X PATCH "$BASE/organizations/$OSLUG/matches/$mid" "${AUTH[@]}" -H 'Content-Type: application/json' -d "{\"status\":\"completed\",\"winner_team_id\":\"$home\"}")
  echo "  match $mid: home $home wins -> status=$(echo "$DONE" | j status) score=$(echo "$DONE" | sed -n 's/.*"home_score":\([0-9]*\).*/\1/p')-$(echo "$DONE" | sed -n 's/.*"away_score":\([0-9]*\).*/\1/p')"
}

for round in 1 2 3; do
  step "13.$round. Score round-$round matches"
  ROWS=$("${PSQL[@]}" -c "SELECT id||','||home_team_id||','||away_team_id FROM matches WHERE tournament_id='$TID' AND status='scheduled' AND home_team_id IS NOT NULL AND away_team_id IS NOT NULL ORDER BY round_number, match_number;")
  [ -z "$ROWS" ] && { echo "  (no more playable matches)"; break; }
  while IFS=, read -r mid home away; do
    [ -z "$mid" ] && continue
    score_match "$mid" "$home"
  done <<< "$ROWS"
done

step "14. ongoing -> completed"
curl -s -o /dev/null -w "  status=%{http_code}\n" -X PATCH "$BASE/organizations/$OSLUG/tournaments/$TID" "${AUTH[@]}" -H 'Content-Type: application/json' -d '{"status":"completed"}'

step "RESULT"
echo "  org slug:        $OSLUG"
echo "  tournament slug: $TSLUG"
echo "  tournament id:   $TID"
echo "=== match summary (from DB) ==="
docker exec -i playarena-postgres psql -U postgres -d playarena -c "SELECT round_number AS rnd, match_number AS m, status, home_score AS h, away_score AS a FROM matches WHERE tournament_id='$TID' ORDER BY round_number, match_number;"
