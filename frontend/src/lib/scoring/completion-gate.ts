import type { LiveScore, MatchStatus, UpdateMatchRequest } from "@/types/api/matches"

/**
 * Completion-gate foundations. The integrity rule for ending a match: a result
 * may only be completed when the match is live, NOTHING is unsynced (every
 * scored point is on the authoritative log), no event was permanently rejected,
 * and the authoritative score is loaded. The winner is then derived from that
 * authoritative score — never from the optimistic local fold.
 *
 * FE-7BB owns this gate (pure, tested). FE-7BC wires the actual completion PATCH
 * behind it.
 */

export interface CompletionWinner {
  side: "home" | "away" | "draw"
  participantId?: string
}

export interface CompletionReadiness {
  ready: boolean
  reason?: string
  unsyncedCount: number
  hasFailed: boolean
  winner: CompletionWinner | null
}

export function evaluateCompletion(args: {
  status: MatchStatus
  unsyncedCount: number
  hasFailed: boolean
  score: LiveScore | null | undefined
}): CompletionReadiness {
  const { status, unsyncedCount, hasFailed, score } = args
  const base = { unsyncedCount, hasFailed, winner: null as CompletionWinner | null }

  if (status !== "live") {
    return { ...base, ready: false, reason: "Match is not live." }
  }
  if (hasFailed) {
    return { ...base, ready: false, reason: "Resolve rejected events before completing." }
  }
  if (unsyncedCount > 0) {
    return {
      ...base,
      ready: false,
      reason: `Sync ${unsyncedCount} event${unsyncedCount === 1 ? "" : "s"} before completing.`,
    }
  }
  if (!score) {
    return { ...base, ready: false, reason: "Loading the authoritative score…" }
  }

  return { ...base, ready: true, winner: computeWinner(score) }
}

function computeWinner(score: LiveScore): CompletionWinner {
  if (score.home_score > score.away_score) {
    return { side: "home", participantId: score.home_team_id ?? score.home_player_id ?? undefined }
  }
  if (score.away_score > score.home_score) {
    return { side: "away", participantId: score.away_team_id ?? score.away_player_id ?? undefined }
  }
  return { side: "draw" }
}

/**
 * Builds the PATCH body for completing a match. The winner MUST agree with the
 * authoritative score or the backend rejects it (ErrWinnerScoreMismatch):
 *   - a decided match sets the winning participant's team/player id;
 *   - a draw sets NO winner.
 */
export function buildCompletionBody(
  isTeam: boolean,
  winner: CompletionWinner,
): UpdateMatchRequest {
  if (winner.side === "draw" || !winner.participantId) {
    return { status: "completed" }
  }
  return isTeam
    ? { status: "completed", winner_team_id: winner.participantId }
    : { status: "completed", winner_player_id: winner.participantId }
}
