import type { Match } from "@/types/api/matches"
import type { MatchEvent } from "@/types/api/match-events"

/**
 * PlayArena live-scoring engine — TypeScript port of the authoritative Go
 * engine (`backend/internal/scoring`).
 *
 * INTEGRITY CONTRACT: this fold must produce the SAME score the backend derives
 * from the same event history (`GET /matches/{id}/score`). The backend is the
 * source of truth; this engine exists for instant, offline-capable display and
 * is verified against the backend rules by golden-vector tests. Any divergence
 * is a bug in this file, never a reason to "fix" the displayed number.
 *
 * Kabaddi scoring rules (mirrors backend `scoring/rules.go`):
 *
 *   raid_successful      payload.points (>0 int)  → raiding team (event.team_id)
 *   bonus_point_awarded  +1                       → raiding team
 *   tackle_successful    +1                       → defending team
 *   super_tackle         +2                       → defending team
 *   all_out              payload.bonus_points     → OPPONENT of payload.team_id (eliminated)
 *   penalty_awarded      payload.points (>0 int)  → attributed team
 *   super_raid           0  (label only; the raid_successful carries the points)
 *   score_correction     0  (carries cancels_event_id; cancels its target)
 *   all other types      0
 *
 * EFFECTIVE SET (mirrors `GetEffectiveMatchEventsForScore`): an event is
 * cancelled — and contributes nothing — when its id appears as some event's
 * `cancels_event_id`. The score_correction events themselves remain in the set
 * but score 0. Pure, deterministic, no side effects.
 */

export type ScoringMatch = Pick<
  Match,
  "home_team_id" | "away_team_id" | "home_player_id" | "away_player_id"
>

export type Side = "home" | "away" | "none"

export interface ComputedScore {
  home: number
  away: number
}

export interface ScoreContribution {
  points: number
  side: Side
}

// Team-format matches attribute by team_id; individual-format by player_id.
// Mirrors the Go engine keying on `match.HomeTeamID.Valid`.
function isTeamMode(m: ScoringMatch): boolean {
  return m.home_team_id != null
}

// Resolves which side an event is attributed to via its participant id.
function participantSide(evt: MatchEvent, m: ScoringMatch): Side {
  if (isTeamMode(m)) {
    const id = evt.team_id
    if (!id) return "none"
    if (id === m.home_team_id) return "home"
    if (id === m.away_team_id) return "away"
    return "none"
  }
  const id = evt.player_id
  if (!id) return "none"
  if (id === m.home_player_id) return "home"
  if (id === m.away_player_id) return "away"
  return "none"
}

// Resolves a raw participant id (from an all_out payload) to a side, checking
// both team and player slots — mirrors the Go `sideByID`.
function sideById(id: string, m: ScoringMatch): Side {
  if (!id) return "none"
  if (m.home_team_id && id === m.home_team_id) return "home"
  if (m.away_team_id && id === m.away_team_id) return "away"
  if (m.home_player_id && id === m.home_player_id) return "home"
  if (m.away_player_id && id === m.away_player_id) return "away"
  return "none"
}

// Extracts payload.points as a positive integer; 0 otherwise. Mirrors the Go
// `payloadPoints` (which rejects missing / non-positive / non-integer values).
function payloadPoints(payload: Record<string, unknown> | null | undefined): number {
  const p = payload?.points
  if (typeof p === "number" && Number.isInteger(p) && p > 0) return p
  return 0
}

// all_out: payload.team_id is the ELIMINATED side; the opponent receives
// payload.bonus_points. Returns (0, none) on any payload anomaly — mirrors Go
// `allOutScore`.
function allOutScore(
  payload: Record<string, unknown> | null | undefined,
  m: ScoringMatch,
): ScoreContribution {
  const teamId = payload?.team_id
  const bonus = payload?.bonus_points
  if (typeof teamId !== "string" || teamId === "") return { points: 0, side: "none" }
  if (typeof bonus !== "number" || !Number.isInteger(bonus) || bonus <= 0) {
    return { points: 0, side: "none" }
  }
  switch (sideById(teamId, m)) {
    case "home":
      return { points: bonus, side: "away" }
    case "away":
      return { points: bonus, side: "home" }
    default:
      return { points: 0, side: "none" }
  }
}

/**
 * Point value and recipient side for a single event. Non-scoring types
 * (lifecycle, super_raid, score_correction, player state, raid_attempt/empty)
 * return 0. Exported for the read-only timeline so per-event deltas use the
 * exact same rules as the score fold.
 */
export function scoreContribution(evt: MatchEvent, m: ScoringMatch): ScoreContribution {
  switch (evt.event_type) {
    case "raid_successful":
      return { points: payloadPoints(evt.payload), side: participantSide(evt, m) }
    case "bonus_point_awarded":
      return { points: 1, side: participantSide(evt, m) }
    case "tackle_successful":
      return { points: 1, side: participantSide(evt, m) }
    case "super_tackle":
      return { points: 2, side: participantSide(evt, m) }
    case "all_out":
      return allOutScore(evt.payload, m)
    case "penalty_awarded":
      return { points: payloadPoints(evt.payload), side: participantSide(evt, m) }
    default:
      return { points: 0, side: "none" }
  }
}

/**
 * The set of event ids cancelled by a score_correction (their id appears as
 * some event's cancels_event_id). Exported so the timeline can mark cancelled
 * events without recomputing.
 */
export function cancelledEventIds(events: MatchEvent[]): Set<string> {
  const cancelled = new Set<string>()
  for (const e of events) {
    if (e.cancels_event_id) cancelled.add(e.cancels_event_id)
  }
  return cancelled
}

/** Events that still count toward the score (cancelled targets removed). */
export function effectiveEvents(events: MatchEvent[]): MatchEvent[] {
  const cancelled = cancelledEventIds(events)
  return events.filter((e) => !cancelled.has(e.id))
}

/**
 * Deterministic score fold over the effective event set. Order-independent for
 * point sums; cancellation is resolved by id, not position — so a raw log in
 * any order yields the same result the backend computes.
 */
export function computeScore(m: ScoringMatch, events: MatchEvent[]): ComputedScore {
  let home = 0
  let away = 0
  for (const e of effectiveEvents(events)) {
    const { points, side } = scoreContribution(e, m)
    if (side === "home") home += points
    else if (side === "away") away += points
  }
  return { home, away }
}
