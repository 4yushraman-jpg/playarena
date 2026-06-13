import { describe, it, expect } from "vitest"
import { buildCompletionBody, type CompletionWinner } from "@/lib/scoring/completion-gate"
import { buildGeneric, buildPenalty } from "@/lib/scoring/scoring-actions"

describe("buildCompletionBody", () => {
  it("sets winner_team_id for a team-format home win", () => {
    const winner: CompletionWinner = { side: "home", participantId: "tm-1" }
    expect(buildCompletionBody(true, winner)).toEqual({ status: "completed", winner_team_id: "tm-1" })
  })

  it("sets winner_player_id for an individual-format away win", () => {
    const winner: CompletionWinner = { side: "away", participantId: "p-2" }
    expect(buildCompletionBody(false, winner)).toEqual({ status: "completed", winner_player_id: "p-2" })
  })

  it("sets NO winner for a draw (backend requires absent winner on equal scores)", () => {
    expect(buildCompletionBody(true, { side: "draw" })).toEqual({ status: "completed" })
  })

  it("omits the winner when the participant id is unknown", () => {
    expect(buildCompletionBody(true, { side: "home" })).toEqual({ status: "completed" })
  })
})

describe("buildGeneric", () => {
  it("builds a non-scoring lifecycle event with a client_event_id and period", () => {
    const a = buildGeneric("half_started", { period: 2 })
    expect(a.isScoring).toBe(false)
    expect(a.body.event_type).toBe("half_started")
    expect(a.body.period).toBe(2)
    expect(a.body.payload).toMatchObject({ client_event_id: a.clientEventId })
  })

  it("carries an optional payload (e.g. timeout kind) and attribution", () => {
    const a = buildGeneric("timeout_called", {
      payload: { kind: "technical" },
      attribution: { teamMode: true, participantId: "tm-1" },
      clockSeconds: 120,
    })
    expect(a.body.team_id).toBe("tm-1")
    expect(a.body.clock_seconds).toBe(120)
    expect(a.body.payload).toMatchObject({ kind: "technical", client_event_id: a.clientEventId })
  })

  it("each generic event gets a distinct client_event_id", () => {
    expect(buildGeneric("player_out").clientEventId).not.toBe(buildGeneric("player_out").clientEventId)
  })
})

describe("buildPenalty (used by the More sheet)", () => {
  it("awards payload.points to the chosen side", () => {
    const a = buildPenalty({ teamMode: true, participantId: "tm-1" }, 2)
    expect(a.body.event_type).toBe("penalty_awarded")
    expect(a.body.team_id).toBe("tm-1")
    expect(a.body.payload).toMatchObject({ points: 2, client_event_id: a.clientEventId })
  })
})
