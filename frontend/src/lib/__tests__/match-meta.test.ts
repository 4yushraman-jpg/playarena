import { describe, it, expect } from "vitest"
import {
  matchParticipantType,
  matchParticipantIds,
  formatMatchLabel,
  isFixtureEditable,
} from "@/lib/match-meta"
import type { Match, MatchStatus } from "@/types/api/matches"

function makeMatch(overrides: Partial<Match> = {}): Match {
  return {
    id: "m1",
    organization_id: "org1",
    tournament_id: "tour1",
    round_number: null,
    round_name: null,
    match_number: null,
    home_team_id: null,
    away_team_id: null,
    home_player_id: null,
    away_player_id: null,
    venue: null,
    scheduled_at: "2026-07-01T10:00:00Z",
    started_at: null,
    ended_at: null,
    status: "scheduled",
    winner_team_id: null,
    winner_player_id: null,
    is_walkover: false,
    home_score: 0,
    away_score: 0,
    notes: null,
    created_at: "2026-06-01T00:00:00Z",
    updated_at: "2026-06-01T00:00:00Z",
    ...overrides,
  }
}

describe("matchParticipantType", () => {
  it("detects team format from team slots", () => {
    expect(matchParticipantType(makeMatch({ home_team_id: "a", away_team_id: "b" }))).toBe("team")
  })
  it("detects individual format from player slots", () => {
    expect(
      matchParticipantType(makeMatch({ home_player_id: "p1", away_player_id: "p2" })),
    ).toBe("individual")
  })
})

describe("matchParticipantIds", () => {
  it("returns team ids for team matches", () => {
    const ids = matchParticipantIds(
      makeMatch({
        home_team_id: "h",
        away_team_id: "a",
        winner_team_id: "h",
        status: "completed",
      }),
    )
    expect(ids).toEqual({ homeId: "h", awayId: "a", winnerId: "h" })
  })
  it("returns player ids for individual matches", () => {
    const ids = matchParticipantIds(
      makeMatch({ home_player_id: "ph", away_player_id: "pa", winner_player_id: "pa" }),
    )
    expect(ids).toEqual({ homeId: "ph", awayId: "pa", winnerId: "pa" })
  })
})

describe("formatMatchLabel", () => {
  it("prefers round name over round number", () => {
    expect(formatMatchLabel(makeMatch({ round_name: "Final", match_number: 1 }))).toBe(
      "Final · Match 1",
    )
  })
  it("falls back to round number", () => {
    expect(formatMatchLabel(makeMatch({ round_number: 2, match_number: 3 }))).toBe(
      "Round 2 · Match 3",
    )
  })
  it("returns 'Match' when no positioning metadata", () => {
    expect(formatMatchLabel(makeMatch())).toBe("Match")
  })
})

describe("isFixtureEditable", () => {
  it("is true only for scheduled matches", () => {
    expect(isFixtureEditable(makeMatch({ status: "scheduled" }))).toBe(true)
    const terminal: MatchStatus[] = ["live", "completed", "cancelled", "abandoned"]
    for (const status of terminal) {
      expect(isFixtureEditable(makeMatch({ status }))).toBe(false)
    }
  })
})
