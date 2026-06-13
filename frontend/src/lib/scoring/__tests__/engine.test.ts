import { describe, it, expect } from "vitest"
import {
  computeScore,
  effectiveEvents,
  cancelledEventIds,
  scoreContribution,
  type ScoringMatch,
} from "@/lib/scoring/engine"
import type { MatchEvent, MatchEventType } from "@/types/api/match-events"

// ── Fixtures ───────────────────────────────────────────────────────────────────

const TEAM_MATCH: ScoringMatch = {
  home_team_id: "home-team",
  away_team_id: "away-team",
  home_player_id: null,
  away_player_id: null,
}

const INDIVIDUAL_MATCH: ScoringMatch = {
  home_team_id: null,
  away_team_id: null,
  home_player_id: "home-player",
  away_player_id: "away-player",
}

let seq = 0
function ev(
  type: MatchEventType,
  opts: Partial<MatchEvent> = {},
): MatchEvent {
  seq += 1
  return {
    id: opts.id ?? `e${seq}`,
    match_id: "m1",
    organization_id: "org1",
    sequence_number: seq,
    event_type: type,
    team_id: opts.team_id ?? null,
    player_id: opts.player_id ?? null,
    period: opts.period ?? 1,
    clock_seconds: opts.clock_seconds ?? null,
    payload: opts.payload ?? {},
    recorded_by: opts.recorded_by ?? "u1",
    recorded_at: "2026-07-01T10:00:00Z",
    cancels_event_id: opts.cancels_event_id ?? null,
    created_at: "2026-07-01T10:00:00Z",
  }
}

// ── Per-rule golden vectors (team mode) ─────────────────────────────────────────

describe("scoring engine — individual rules (team mode)", () => {
  it("raid_successful adds payload.points to the raiding team", () => {
    expect(computeScore(TEAM_MATCH, [
      ev("raid_successful", { team_id: "home-team", payload: { points: 3 } }),
    ])).toEqual({ home: 3, away: 0 })
  })

  it("raid_successful with missing/zero/negative/non-integer points adds 0", () => {
    for (const payload of [{}, { points: 0 }, { points: -2 }, { points: 1.5 }, { points: "2" }]) {
      expect(computeScore(TEAM_MATCH, [
        ev("raid_successful", { team_id: "home-team", payload: payload as Record<string, unknown> }),
      ])).toEqual({ home: 0, away: 0 })
    }
  })

  it("bonus_point_awarded adds 1 to the raiding team", () => {
    expect(computeScore(TEAM_MATCH, [
      ev("bonus_point_awarded", { team_id: "away-team" }),
    ])).toEqual({ home: 0, away: 1 })
  })

  it("tackle_successful adds 1; super_tackle adds 2 to the defending team", () => {
    expect(computeScore(TEAM_MATCH, [
      ev("tackle_successful", { team_id: "home-team" }),
      ev("super_tackle", { team_id: "home-team" }),
    ])).toEqual({ home: 3, away: 0 })
  })

  it("penalty_awarded adds payload.points to the attributed team", () => {
    expect(computeScore(TEAM_MATCH, [
      ev("penalty_awarded", { team_id: "away-team", payload: { points: 2 } }),
    ])).toEqual({ home: 0, away: 2 })
  })

  it("super_raid contributes 0 (label only — raid_successful carries the points)", () => {
    expect(computeScore(TEAM_MATCH, [
      ev("super_raid", { team_id: "home-team", payload: { points: 3 } }),
    ])).toEqual({ home: 0, away: 0 })
  })

  it("lifecycle / player-state / raid_attempt / raid_empty contribute 0", () => {
    const noop: MatchEventType[] = [
      "match_started", "half_started", "timeout_called", "raid_attempt",
      "raid_empty", "do_or_die_raid", "player_out", "player_revived",
      "player_substituted", "player_injured",
    ]
    expect(computeScore(TEAM_MATCH, noop.map((t) => ev(t, { team_id: "home-team" }))))
      .toEqual({ home: 0, away: 0 })
  })

  it("a scoring event whose team_id is not a participant contributes 0", () => {
    expect(computeScore(TEAM_MATCH, [
      ev("raid_successful", { team_id: "stranger-team", payload: { points: 5 } }),
    ])).toEqual({ home: 0, away: 0 })
  })
})

// ── all_out (the inverted-attribution rule) ─────────────────────────────────────

describe("scoring engine — all_out", () => {
  it("awards bonus_points to the OPPONENT of the eliminated team", () => {
    expect(computeScore(TEAM_MATCH, [
      ev("all_out", { team_id: "home-team", payload: { team_id: "home-team", bonus_points: 2 } }),
    ])).toEqual({ home: 0, away: 2 })

    expect(computeScore(TEAM_MATCH, [
      ev("all_out", { team_id: "away-team", payload: { team_id: "away-team", bonus_points: 2 } }),
    ])).toEqual({ home: 2, away: 0 })
  })

  it("contributes 0 when payload.team_id is missing, non-participant, or bonus_points invalid", () => {
    const bad: Record<string, unknown>[] = [
      {},
      { team_id: "", bonus_points: 2 },
      { team_id: "stranger", bonus_points: 2 },
      { team_id: "home-team" },
      { team_id: "home-team", bonus_points: 0 },
      { team_id: "home-team", bonus_points: -1 },
    ]
    for (const payload of bad) {
      expect(computeScore(TEAM_MATCH, [ev("all_out", { payload })])).toEqual({ home: 0, away: 0 })
    }
  })
})

// ── Corrections (effective-set semantics) ───────────────────────────────────────

describe("scoring engine — score_correction", () => {
  it("a correction removes its target's points and itself contributes 0", () => {
    const raid = ev("raid_successful", { id: "raid-1", team_id: "home-team", payload: { points: 3 } })
    const correction = ev("score_correction", { id: "corr-1", cancels_event_id: "raid-1" })
    expect(computeScore(TEAM_MATCH, [raid, correction])).toEqual({ home: 0, away: 0 })
  })

  it("only the cancelled target is removed; other events still count", () => {
    const events = [
      ev("raid_successful", { id: "r1", team_id: "home-team", payload: { points: 2 } }),
      ev("tackle_successful", { id: "t1", team_id: "away-team" }),
      ev("raid_successful", { id: "r2", team_id: "home-team", payload: { points: 3 } }),
      ev("score_correction", { id: "c1", cancels_event_id: "r2" }),
    ]
    expect(computeScore(TEAM_MATCH, events)).toEqual({ home: 2, away: 1 })
  })

  it("cancellation is resolved by id, independent of event order", () => {
    const ordered = [
      ev("raid_successful", { id: "rX", team_id: "away-team", payload: { points: 2 } }),
      ev("score_correction", { id: "cX", cancels_event_id: "rX" }),
    ]
    const reversed = [...ordered].reverse()
    expect(computeScore(TEAM_MATCH, reversed)).toEqual(computeScore(TEAM_MATCH, ordered))
    expect(computeScore(TEAM_MATCH, reversed)).toEqual({ home: 0, away: 0 })
  })

  it("cancelledEventIds and effectiveEvents expose the correct effective set", () => {
    const events = [
      ev("raid_successful", { id: "r1", team_id: "home-team", payload: { points: 2 } }),
      ev("score_correction", { id: "c1", cancels_event_id: "r1" }),
    ]
    expect(cancelledEventIds(events)).toEqual(new Set(["r1"]))
    // The cancelled target is dropped; the correction itself remains (scores 0).
    expect(effectiveEvents(events).map((e) => e.id)).toEqual(["c1"])
  })
})

// ── Individual (player) mode ────────────────────────────────────────────────────

describe("scoring engine — individual mode", () => {
  it("attributes by player_id when the match is individual-format", () => {
    expect(computeScore(INDIVIDUAL_MATCH, [
      ev("raid_successful", { player_id: "home-player", payload: { points: 2 } }),
      ev("super_tackle", { player_id: "away-player" }),
    ])).toEqual({ home: 2, away: 2 })
  })

  it("ignores team_id in individual mode", () => {
    expect(computeScore(INDIVIDUAL_MATCH, [
      ev("raid_successful", { team_id: "home-team", payload: { points: 5 } }),
    ])).toEqual({ home: 0, away: 0 })
  })
})

// ── Realistic full-match golden vector ──────────────────────────────────────────

describe("scoring engine — full-match golden vector", () => {
  it("derives the correct final score from a representative sequence", () => {
    const events: MatchEvent[] = [
      ev("match_started"),
      ev("half_started", { period: 1 }),
      ev("raid_successful", { id: "r1", team_id: "home-team", payload: { points: 1 } }), // H1
      ev("tackle_successful", { id: "t1", team_id: "away-team" }),                       // A1
      ev("raid_successful", { id: "r2", team_id: "home-team", payload: { points: 2 } }), // H3
      ev("bonus_point_awarded", { id: "b1", team_id: "home-team" }),                     // H4
      ev("super_tackle", { id: "st1", team_id: "away-team" }),                           // A3
      ev("all_out", { id: "ao1", team_id: "away-team", payload: { team_id: "away-team", bonus_points: 2 } }), // H6
      ev("raid_successful", { id: "r3", team_id: "away-team", payload: { points: 3 } }), // A6
      ev("score_correction", { id: "c1", cancels_event_id: "r3" }),                      // cancel A6 → A3
      ev("penalty_awarded", { id: "p1", team_id: "home-team", payload: { points: 1 } }), // H7
      ev("half_ended", { period: 1 }),
      ev("match_ended"),
    ]
    // Home: 1 + 2 + 1(bonus) + 2(all_out) + 1(penalty) = 7
    // Away: 1(tackle) + 2(super_tackle) + [3 raid cancelled] = 3
    expect(computeScore(TEAM_MATCH, events)).toEqual({ home: 7, away: 3 })
  })
})

// ── scoreContribution (timeline helper) ─────────────────────────────────────────

describe("scoreContribution", () => {
  it("reports per-event delta + side using the same rules as the fold", () => {
    expect(scoreContribution(
      ev("super_tackle", { team_id: "home-team" }), TEAM_MATCH,
    )).toEqual({ points: 2, side: "home" })
    expect(scoreContribution(
      ev("all_out", { payload: { team_id: "home-team", bonus_points: 2 } }), TEAM_MATCH,
    )).toEqual({ points: 2, side: "away" })
    expect(scoreContribution(ev("match_started"), TEAM_MATCH)).toEqual({ points: 0, side: "none" })
  })
})
