-- Matches queries
-- organization_id is always required to enforce tenant isolation.

-- name: GetMatchByID :one
SELECT *
FROM   matches
WHERE  id              = $1
  AND  organization_id = $2
LIMIT  1;

-- name: ListMatchesByTournament :many
SELECT *
FROM   matches
WHERE  tournament_id   = $1
  AND  organization_id = $2
ORDER  BY round_number ASC NULLS LAST,
          match_number  ASC NULLS LAST;
