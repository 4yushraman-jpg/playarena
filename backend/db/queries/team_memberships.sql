-- Team membership queries
-- organization_id is always required to enforce tenant isolation.
-- All three of team_id, player_id, and organization_id must be consistent
-- (enforced by both the application service layer and the DB trigger
-- trg_team_memberships_org_consistency).

-- name: GetMembershipByID :one
SELECT *
FROM   team_memberships
WHERE  id              = $1
  AND  organization_id = $2
LIMIT  1;

-- name: GetActiveMembershipByTeamAndPlayer :one
-- Returns the current active membership for a (team, player) pair.
-- Retained for internal use; the primary exclusivity check is
-- GetActiveMembershipByPlayer which enforces the org-wide rule.
SELECT *
FROM   team_memberships
WHERE  team_id         = $1
  AND  player_id       = $2
  AND  organization_id = $3
  AND  status          = 'active'
LIMIT  1;

-- name: GetActiveMembershipByPlayer :one
-- Enforces the rule: one active team membership per player per organization.
-- Does NOT filter by team_id so that any existing active membership on any
-- team within the org is detected before a new one is created.
SELECT *
FROM   team_memberships
WHERE  player_id       = $1
  AND  organization_id = $2
  AND  status          = 'active'
LIMIT  1;

-- name: ListActiveMembersByTeam :many
-- Returns all currently active members of a team ordered by join date.
-- Excludes historical (released, transferred, inactive) memberships.
SELECT *
FROM   team_memberships
WHERE  team_id         = $1
  AND  organization_id = $2
  AND  status          = 'active'
ORDER  BY joined_at ASC;

-- name: CreateMembership :one
-- Inserts a new active membership row. The DB trigger
-- trg_team_memberships_org_consistency validates that team_id and player_id
-- both belong to organization_id; the service layer validates this first to
-- return a meaningful error before hitting the trigger.
INSERT INTO team_memberships (
    team_id, player_id, organization_id,
    role, jersey_number, notes
)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: HasActiveMembersByTeam :one
-- Returns true if the team has at least one active member.
-- Used by tournament registration to reject empty teams.
SELECT EXISTS(
    SELECT 1
    FROM   team_memberships
    WHERE  team_id         = $1
      AND  organization_id = $2
      AND  status          = 'active'
) AS has_members;

-- name: RemoveMembership :one
-- Soft-removes by setting status = 'released' and left_at = NOW().
-- Scoped by id, team_id, and organization_id to prevent cross-team or
-- cross-org removal. Only active memberships are affected; returns no rows
-- if the membership is already removed or belongs to a different team/org.
UPDATE team_memberships
SET    status     = 'released',
       left_at    = NOW(),
       updated_at = NOW()
WHERE  id              = $1
  AND  team_id         = $2
  AND  organization_id = $3
  AND  status          = 'active'
RETURNING *;
