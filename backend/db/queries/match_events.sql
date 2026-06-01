-- Match Events queries
-- organization_id is always required to enforce tenant isolation.
-- This table is append-only: no UPDATE or DELETE queries are ever added.
-- sequence_number is always computed server-side inside a FOR UPDATE transaction.

-- name: LockMatchForUpdate :one
-- Acquires an exclusive row-level lock on the match row for the duration of
-- the event-insert transaction. The lock serialises all concurrent event inserts
-- for the same match, making MAX(sequence_number)+1 and match-status validation
-- race-free. Also returns participant fields needed for in-transaction validation.
SELECT id,
       organization_id,
       status,
       home_team_id,
       away_team_id,
       home_player_id,
       away_player_id
FROM   matches
WHERE  id              = $1
  AND  organization_id = $2
FOR    UPDATE;

-- name: GetMaxSequenceNumber :one
-- Returns the current maximum sequence_number for a match (0 when no events
-- exist yet). Must be called inside the FOR UPDATE transaction so the lock on
-- the parent match row prevents a concurrent insert from changing this value
-- before the new INSERT executes.
SELECT COALESCE(MAX(sequence_number), 0)::bigint
FROM   match_events
WHERE  match_id = $1;

-- name: CreateMatchEvent :one
INSERT INTO match_events (
    match_id,
    organization_id,
    sequence_number,
    event_type,
    team_id,
    player_id,
    period,
    clock_seconds,
    payload,
    recorded_by,
    recorded_at,
    cancels_event_id
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
RETURNING *;

-- name: GetMatchEventByID :one
-- Fetches a single event scoped to its organization. Org-scoping prevents
-- reading events across tenant boundaries.
SELECT *
FROM   match_events
WHERE  id              = $1
  AND  organization_id = $2
LIMIT  1;

-- name: GetMatchEventByMatchAndID :one
-- Fetches a single event scoped to its match AND organization.
-- Enforces the URL resource hierarchy: the event must belong to the match
-- specified in the URL, not just to the organization. Returns ErrNoRows
-- (mapped to ErrEventNotFound) when the event exists in the org but belongs
-- to a different match — preventing cross-match event retrieval.
SELECT *
FROM   match_events
WHERE  id              = $1
  AND  match_id        = $2
  AND  organization_id = $3
LIMIT  1;

-- name: ListMatchEventsByMatch :many
-- Raw event timeline in strict sequence order. Includes all events, even
-- those cancelled by a score_correction. Used for audit trail and replay.
SELECT *
FROM   match_events
WHERE  match_id        = sqlc.arg(match_id)
  AND  organization_id = sqlc.arg(organization_id)
ORDER  BY sequence_number ASC
LIMIT  sqlc.arg(page_limit)
OFFSET sqlc.arg(page_offset);

-- name: ListEffectiveMatchEventsByMatch :many
-- Effective event timeline: excludes events that have been cancelled by a
-- score_correction. An event is cancelled when its id appears in any
-- cancels_event_id within the same match. Use this view for score derivation
-- and live display. The raw timeline (ListMatchEventsByMatch) is available for
-- audit and correction history.
SELECT me.*
FROM   match_events me
WHERE  me.match_id        = sqlc.arg(match_id)
  AND  me.organization_id = sqlc.arg(organization_id)
  AND  me.id NOT IN (
           SELECT c.cancels_event_id
           FROM   match_events c
           WHERE  c.match_id         = sqlc.arg(match_id)
             AND  c.cancels_event_id IS NOT NULL
       )
ORDER  BY me.sequence_number ASC
LIMIT  sqlc.arg(page_limit)
OFFSET sqlc.arg(page_offset);

-- name: CountMatchEventsByMatch :one
-- Total event count (raw) for pagination metadata.
SELECT COUNT(*)
FROM   match_events
WHERE  match_id        = sqlc.arg(match_id)
  AND  organization_id = sqlc.arg(organization_id);

-- name: CountEffectiveMatchEventsByMatch :one
-- Effective event count (excludes cancelled) for pagination metadata.
SELECT COUNT(*)
FROM   match_events me
WHERE  me.match_id        = sqlc.arg(match_id)
  AND  me.organization_id = sqlc.arg(organization_id)
  AND  me.id NOT IN (
           SELECT c.cancels_event_id
           FROM   match_events c
           WHERE  c.match_id         = sqlc.arg(match_id)
             AND  c.cancels_event_id IS NOT NULL
       );

-- name: CountMatchEventsByType :one
-- Returns the number of events of a specific type for a match. Used inside the
-- FOR UPDATE transaction to enforce lifecycle event uniqueness (only one
-- match_started and one match_ended allowed per match).
SELECT COUNT(*)
FROM   match_events
WHERE  match_id   = sqlc.arg(match_id)
  AND  event_type = sqlc.arg(event_type);

-- name: GetMatchEventForCorrection :one
-- Fetches the minimal fields needed to validate a score_correction target:
-- existence, same-match check, and type check (correction chains are forbidden).
-- Does not scope by organization_id: cross-match detection provides the
-- isolation boundary (no two matches share a UUID).
SELECT id, match_id, event_type
FROM   match_events
WHERE  id = $1
LIMIT  1;

-- name: GetEventCancellation :one
-- Returns the id of the score_correction event that has already cancelled the
-- specified target event. If found, the target is already cancelled and a
-- second cancellation must be rejected. ErrNoRows means the target is not yet
-- cancelled and the correction may proceed.
SELECT id
FROM   match_events
WHERE  match_id         = sqlc.arg(match_id)
  AND  cancels_event_id = sqlc.arg(cancels_event_id)
LIMIT  1;

-- name: IsPlayerOnParticipatingTeam :one
-- Returns true if the player has an active team_membership on either the home
-- or away team of a match. Used in team-format matches to validate that a
-- player_id in an event belongs to one of the match participants.
SELECT EXISTS(
    SELECT 1
    FROM   team_memberships
    WHERE  player_id  = sqlc.arg(player_id)
      AND  (team_id   = sqlc.arg(home_team_id)
            OR team_id = sqlc.arg(away_team_id))
      AND  status     = 'active'
) AS on_team;

-- name: IsPlayerOnTeam :one
-- Returns true if the player has an active team_membership on the specified
-- team. Used when both team_id and player_id are present in a single event to
-- verify the player actually belongs to the stated team.
SELECT EXISTS(
    SELECT 1
    FROM   team_memberships
    WHERE  player_id = sqlc.arg(player_id)
      AND  team_id   = sqlc.arg(team_id)
      AND  status    = 'active'
) AS on_team;

-- name: GetEffectiveMatchEventsForScore :many
-- Returns the complete effective event timeline for a match in sequence order.
-- No pagination: the scoring engine requires the full timeline to compute scores.
-- An event is excluded (cancelled) when its id appears in any cancels_event_id
-- within the same match.  score_correction events themselves remain in the
-- result — they contribute zero points and carry the cancels_event_id reference.
SELECT me.*
FROM   match_events me
WHERE  me.match_id        = sqlc.arg(match_id)
  AND  me.organization_id = sqlc.arg(organization_id)
  AND  me.id NOT IN (
           SELECT c.cancels_event_id
           FROM   match_events c
           WHERE  c.match_id         = sqlc.arg(match_id)
             AND  c.cancels_event_id IS NOT NULL
       )
ORDER  BY me.sequence_number ASC;
