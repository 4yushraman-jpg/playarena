-- Public (unauthenticated) read queries — PRI-1.
--
-- These are the ONLY queries the anonymous public path may use. Authorization is
-- expressed entirely through SQL visibility constraints — never a principal
-- check. Every query selects an explicit whitelist of columns: NO "SELECT *",
-- NO settings/audit/PII/internal-notes columns are ever exposed.
--
-- Public eligibility gate (repeated in each entry query):
--   tournament.status <> 'draft'
--   AND tournament.visibility IN ('unlisted','public')
--   AND owning organization.status = 'active'
-- A row failing the gate is simply not returned → the service maps the empty
-- result to 404 (never 403), so private/draft tournaments cannot be probed for
-- existence.

-- name: GetPublicTournament :one
-- Resolves a publicly-eligible tournament by (org slug, tournament slug).
-- Returns only whitelisted, shareable fields plus org branding (name/slug).
-- Note the explicit absence of settings, created_by, and updated_by.
SELECT t.id,
       t.name,
       t.slug,
       t.sport,
       t.format,
       t.participant_type,
       t.status,
       t.visibility,
       t.banner_url,
       t.description,
       t.prize_pool,
       t.currency,
       t.max_participants,
       t.registration_opens_at,
       t.registration_closes_at,
       t.starts_at,
       t.ends_at,
       t.venue,
       t.city,
       t.country,
       t.rules,
       t.created_at,
       t.organization_id,
       o.name AS organization_name,
       o.slug AS organization_slug
FROM   tournaments t
JOIN   organizations o ON o.id = t.organization_id
WHERE  o.slug       = $1
  AND  t.slug       = $2
  AND  t.status    <> 'draft'
  AND  t.visibility IN ('unlisted', 'public')
  AND  o.status     = 'active'
LIMIT  1;

-- name: ListPublicMatches :many
-- Whitelisted match rows for a (publicly-resolved) tournament. Excludes notes,
-- metadata, created_by/recorded_by. Participant/winner names are resolved by the
-- service via the batch name queries — ids here are non-secret render handles.
SELECT id,
       round_number,
       round_name,
       match_number,
       group_label,
       home_team_id,
       away_team_id,
       home_player_id,
       away_player_id,
       scheduled_at,
       started_at,
       ended_at,
       venue,
       status,
       home_score,
       away_score,
       is_walkover,
       winner_team_id,
       winner_player_id,
       next_match_id,
       next_match_slot
FROM   matches
WHERE  tournament_id   = $1
  AND  organization_id = $2
ORDER  BY round_number ASC NULLS LAST, match_number ASC NULLS LAST;

-- name: GetPublicMatch :one
-- A single whitelisted match within a publicly-resolved tournament.
SELECT id,
       round_number,
       round_name,
       match_number,
       group_label,
       home_team_id,
       away_team_id,
       home_player_id,
       away_player_id,
       scheduled_at,
       started_at,
       ended_at,
       venue,
       status,
       home_score,
       away_score,
       is_walkover,
       winner_team_id,
       winner_player_id
FROM   matches
WHERE  id              = $1
  AND  tournament_id   = $2
  AND  organization_id = $3
LIMIT  1;

-- name: ListPublicMatchEvents :many
-- Whitelisted, effective (non-cancelled) timeline events for a public match.
-- Excludes payload, recorded_by, cancels_event_id. Capped to keep the public
-- response bounded.
SELECT me.id,
       me.sequence_number,
       me.event_type,
       me.period,
       me.clock_seconds,
       me.team_id,
       me.player_id,
       me.recorded_at
FROM   match_events me
WHERE  me.match_id        = $1
  AND  me.organization_id = $2
  AND  me.event_type     <> 'score_correction'
  AND  me.id NOT IN (
           SELECT c.cancels_event_id
           FROM   match_events c
           WHERE  c.match_id         = $1
             AND  c.cancels_event_id IS NOT NULL
       )
ORDER  BY me.sequence_number ASC
LIMIT  500;
