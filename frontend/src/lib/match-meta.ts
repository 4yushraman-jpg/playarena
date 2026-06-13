import type { Match } from "@/types/api/matches"

// ── Pure helpers for match presentation ───────────────────────────────────────
// No React, no API — kept here so they are trivially unit-testable and shared
// across the list, detail, and fixtures surfaces.

export type MatchParticipantType = "team" | "individual"

/**
 * Determines whether a match is team-format or individual-format from its
 * participant slots. A scheduled fixture always has both slots filled for its
 * format; we key on the home slot and fall back to away in case one side is
 * still unset.
 */
export function matchParticipantType(match: Match): MatchParticipantType {
  if (match.home_team_id || match.away_team_id) return "team"
  if (match.home_player_id || match.away_player_id) return "individual"
  return "team"
}

export interface MatchParticipantIds {
  homeId: string | null
  awayId: string | null
  winnerId: string | null
}

/**
 * Resolves the home/away/winner participant UUIDs regardless of format,
 * picking the team slots for team matches and the player slots otherwise.
 */
export function matchParticipantIds(match: Match): MatchParticipantIds {
  if (matchParticipantType(match) === "team") {
    return {
      homeId: match.home_team_id,
      awayId: match.away_team_id,
      winnerId: match.winner_team_id,
    }
  }
  return {
    homeId: match.home_player_id,
    awayId: match.away_player_id,
    winnerId: match.winner_player_id,
  }
}

/**
 * A short human label for a fixture's round/match position, e.g.
 * "Round 2 · Match 3", "Quarter-final", or "Match 3". Returns "Match" when no
 * positioning metadata is present.
 */
export function formatMatchLabel(match: Match): string {
  const parts: string[] = []
  if (match.round_name) {
    parts.push(match.round_name)
  } else if (match.round_number != null) {
    parts.push(`Round ${match.round_number}`)
  }
  if (match.match_number != null) {
    parts.push(`Match ${match.match_number}`)
  }
  return parts.length > 0 ? parts.join(" · ") : "Match"
}

/**
 * Only scheduled fixtures may be edited or cancelled. Once a match is live or
 * terminal (completed/cancelled/abandoned) it is managed by the live-scoring
 * surface (FE-7B), not fixture management.
 */
export function isFixtureEditable(match: Match): boolean {
  return match.status === "scheduled"
}
