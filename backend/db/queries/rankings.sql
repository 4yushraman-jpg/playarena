-- Rankings queries
-- Upserts are called at tournament completion (idempotent on retry).
-- List queries aggregate per-tournament rows and apply RANK() window function.

-- name: UpsertPlayerTournamentStats :exec
-- Idempotent upsert: re-running on retry produces the same result.
INSERT INTO player_tournament_stats (
    player_id, tournament_id, organization_id,
    position, matches_played, matches_won, matches_drawn, matches_lost,
    points, score_for, score_against, snapshotted_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, NOW()
)
ON CONFLICT (player_id, tournament_id) DO UPDATE SET
    organization_id = EXCLUDED.organization_id,
    position        = EXCLUDED.position,
    matches_played  = EXCLUDED.matches_played,
    matches_won     = EXCLUDED.matches_won,
    matches_drawn   = EXCLUDED.matches_drawn,
    matches_lost    = EXCLUDED.matches_lost,
    points          = EXCLUDED.points,
    score_for       = EXCLUDED.score_for,
    score_against   = EXCLUDED.score_against,
    snapshotted_at  = NOW();

-- name: UpsertTeamTournamentStats :exec
INSERT INTO team_tournament_stats (
    team_id, tournament_id, organization_id,
    position, matches_played, matches_won, matches_drawn, matches_lost,
    points, score_for, score_against, snapshotted_at
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, NOW()
)
ON CONFLICT (team_id, tournament_id) DO UPDATE SET
    organization_id = EXCLUDED.organization_id,
    position        = EXCLUDED.position,
    matches_played  = EXCLUDED.matches_played,
    matches_won     = EXCLUDED.matches_won,
    matches_drawn   = EXCLUDED.matches_drawn,
    matches_lost    = EXCLUDED.matches_lost,
    points          = EXCLUDED.points,
    score_for       = EXCLUDED.score_for,
    score_against   = EXCLUDED.score_against,
    snapshotted_at  = NOW();

-- name: ListPlayerRankings :many
-- Returns all-time player rankings for an organization, ordered by the
-- five-level tiebreak chain. Only active players are included.
-- win_rate is computed in the service layer to avoid float precision issues.
WITH aggregated AS (
    SELECT
        p.id                                                              AS player_id,
        p.display_name                                                    AS display_name,
        COUNT(DISTINCT pts.tournament_id)                                 AS tournaments_played,
        SUM(CASE WHEN pts.position = 1  THEN 1 ELSE 0 END)               AS tournaments_won,
        SUM(CASE WHEN pts.position <= 3 THEN 1 ELSE 0 END)               AS podium_finishes,
        SUM(pts.matches_played)                                           AS total_matches,
        SUM(pts.matches_won)                                              AS total_wins,
        SUM(pts.matches_drawn)                                            AS total_draws,
        SUM(pts.matches_lost)                                             AS total_losses,
        SUM(pts.points)                                                   AS total_points,
        SUM(pts.score_for)                                                AS score_for,
        SUM(pts.score_against)                                            AS score_against
    FROM   player_tournament_stats pts
    JOIN   players p ON p.id = pts.player_id
    WHERE  pts.organization_id = $1
      AND  p.status = 'active'
    GROUP  BY p.id, p.display_name
)
SELECT
    RANK() OVER (
        ORDER BY
            tournaments_won  DESC,
            podium_finishes  DESC,
            total_points     DESC,
            total_wins       DESC,
            total_matches    DESC
    )::int                   AS rank,
    player_id,
    display_name,
    tournaments_played::int  AS tournaments_played,
    tournaments_won::int     AS tournaments_won,
    podium_finishes::int     AS podium_finishes,
    total_matches::int       AS total_matches,
    total_wins::int          AS total_wins,
    total_draws::int         AS total_draws,
    total_losses::int        AS total_losses,
    total_points::int        AS total_points,
    score_for::int           AS score_for,
    score_against::int       AS score_against
FROM   aggregated
ORDER  BY rank ASC, player_id ASC
LIMIT  $2
OFFSET $3;

-- name: CountPlayerRankings :one
-- Returns total number of active players with at least one completed tournament
-- in the organization. Used for pagination metadata.
SELECT COUNT(DISTINCT pts.player_id)
FROM   player_tournament_stats pts
JOIN   players p ON p.id = pts.player_id
WHERE  pts.organization_id = $1
  AND  p.status = 'active';

-- name: ListTeamRankings :many
WITH aggregated AS (
    SELECT
        t.id                                                              AS team_id,
        t.name                                                            AS team_name,
        COUNT(DISTINCT tts.tournament_id)                                 AS tournaments_played,
        SUM(CASE WHEN tts.position = 1  THEN 1 ELSE 0 END)               AS tournaments_won,
        SUM(CASE WHEN tts.position <= 3 THEN 1 ELSE 0 END)               AS podium_finishes,
        SUM(tts.matches_played)                                           AS total_matches,
        SUM(tts.matches_won)                                              AS total_wins,
        SUM(tts.matches_drawn)                                            AS total_draws,
        SUM(tts.matches_lost)                                             AS total_losses,
        SUM(tts.points)                                                   AS total_points,
        SUM(tts.score_for)                                                AS score_for,
        SUM(tts.score_against)                                            AS score_against
    FROM   team_tournament_stats tts
    JOIN   teams t ON t.id = tts.team_id
    WHERE  tts.organization_id = $1
      AND  t.status = 'active'
    GROUP  BY t.id, t.name
)
SELECT
    RANK() OVER (
        ORDER BY
            tournaments_won  DESC,
            podium_finishes  DESC,
            total_points     DESC,
            total_wins       DESC,
            total_matches    DESC
    )::int                   AS rank,
    team_id,
    team_name,
    tournaments_played::int  AS tournaments_played,
    tournaments_won::int     AS tournaments_won,
    podium_finishes::int     AS podium_finishes,
    total_matches::int       AS total_matches,
    total_wins::int          AS total_wins,
    total_draws::int         AS total_draws,
    total_losses::int        AS total_losses,
    total_points::int        AS total_points,
    score_for::int           AS score_for,
    score_against::int       AS score_against
FROM   aggregated
ORDER  BY rank ASC, team_id ASC
LIMIT  $2
OFFSET $3;

-- name: CountTeamRankings :one
SELECT COUNT(DISTINCT tts.team_id)
FROM   team_tournament_stats tts
JOIN   teams t ON t.id = tts.team_id
WHERE  tts.organization_id = $1
  AND  t.status = 'active';
