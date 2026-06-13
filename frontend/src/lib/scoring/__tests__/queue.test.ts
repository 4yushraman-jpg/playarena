import { describe, it, expect, beforeEach } from "vitest"
import {
  queueReducer,
  unsyncedCount,
  firstPending,
  firstNeedsReconcile,
  isSending,
  hasFailedPermanent,
  selectUndoTarget,
  type QueueEvent,
} from "@/lib/scoring/queue-reducer"
import { EMPTY_QUEUE_STATE, type QueueState } from "@/lib/scoring/queue-types"
import {
  buildRaid,
  buildTackle,
  buildAllOut,
  buildCorrection,
  type BuiltAction,
} from "@/lib/scoring/scoring-actions"
import { buildConfirmedMap, serverClientIds, serverCancelledIds } from "@/lib/scoring/reconcile"
import { optimisticScore } from "@/lib/scoring/optimistic-score"
import { evaluateCompletion } from "@/lib/scoring/completion-gate"
import { loadQueue, saveQueue, clearQueue } from "@/lib/scoring/queue-storage"
import type { ScoringMatch } from "@/lib/scoring/engine"
import type { MatchEvent } from "@/types/api/match-events"
import type { LiveScore } from "@/types/api/matches"

const HOME = "home-team"
const AWAY = "away-team"
const MATCH: ScoringMatch = { home_team_id: HOME, away_team_id: AWAY, home_player_id: null, away_player_id: null }
const homeAttr = { teamMode: true, participantId: HOME }
const awayAttr = { teamMode: true, participantId: AWAY }

function reduce(state: QueueState, ...events: QueueEvent[]): QueueState {
  return events.reduce(queueReducer, state)
}

function enqueue(state: QueueState, built: BuiltAction, now = 1): QueueState {
  return queueReducer(state, { type: "ENQUEUE", built, now })
}

// A server event mirroring a confirmed action (carries its client_event_id).
function serverEventFrom(
  built: BuiltAction,
  serverId: string,
  seq: number,
): MatchEvent {
  return {
    id: serverId, match_id: "m", organization_id: "o", sequence_number: seq,
    event_type: built.body.event_type, team_id: built.body.team_id ?? null,
    player_id: built.body.player_id ?? null, period: built.body.period ?? 1, clock_seconds: null,
    payload: (built.body.payload ?? {}) as Record<string, unknown>,
    recorded_by: "u", recorded_at: "2026-07-01T10:00:00Z",
    cancels_event_id: built.body.cancels_event_id ?? null, created_at: "2026-07-01T10:00:00Z",
  }
}

// ── scoring-actions ──────────────────────────────────────────────────────────

describe("scoring-actions — intent → event body", () => {
  it("routes to the team slot and embeds client_event_id", () => {
    const a = buildRaid(homeAttr, 3)
    expect(a.body.event_type).toBe("raid_successful")
    expect(a.body.team_id).toBe(HOME)
    expect(a.body.player_id).toBeUndefined()
    expect(a.body.payload).toMatchObject({ points: 3, client_event_id: a.clientEventId })
    expect(a.isScoring).toBe(true)
  })

  it("routes to the player slot in individual mode", () => {
    const a = buildTackle({ teamMode: false, participantId: "p1" })
    expect(a.body.player_id).toBe("p1")
    expect(a.body.team_id).toBeUndefined()
  })

  it("all_out attributes to the eliminated side and carries bonus_points", () => {
    const a = buildAllOut(awayAttr, 2)
    expect(a.body.event_type).toBe("all_out")
    expect(a.body.payload).toMatchObject({ team_id: AWAY, bonus_points: 2, client_event_id: a.clientEventId })
  })

  it("correction references the target server id and is not a scoring action", () => {
    const a = buildCorrection("srv-1")
    expect(a.body.event_type).toBe("score_correction")
    expect(a.body.cancels_event_id).toBe("srv-1")
    expect(a.cancelsServerId).toBe("srv-1")
    expect(a.isScoring).toBe(false)
  })

  it("each action gets a distinct client_event_id", () => {
    const ids = new Set([buildRaid(homeAttr, 1).clientEventId, buildRaid(homeAttr, 1).clientEventId, buildBonusId()])
    expect(ids.size).toBe(3)
  })
})
function buildBonusId() { return buildRaid(homeAttr, 1).clientEventId }

// ── reducer: lifecycle & invariants ─────────────────────────────────────────

describe("queueReducer — enqueue & transitions", () => {
  it("enqueues as pending with a monotonic localSeq", () => {
    let s = enqueue(EMPTY_QUEUE_STATE, buildRaid(homeAttr, 2))
    s = enqueue(s, buildTackle(awayAttr))
    expect(s.actions.map((a) => a.localSeq)).toEqual([1, 2])
    expect(s.actions.every((a) => a.status === "pending")).toBe(true)
    expect(unsyncedCount(s)).toBe(2)
  })

  it("send → confirm marks synced and records the server id", () => {
    const raid = buildRaid(homeAttr, 2)
    let s = enqueue(EMPTY_QUEUE_STATE, raid)
    s = reduce(s,
      { type: "MARK_SENDING", clientEventId: raid.clientEventId },
      { type: "MARK_CONFIRMED", clientEventId: raid.clientEventId, serverId: "S1", serverSequence: 1 },
    )
    expect(s.actions[0].status).toBe("confirmed")
    expect(s.actions[0].serverId).toBe("S1")
    expect(unsyncedCount(s)).toBe(0)
  })

  it("only one action may be sending (single-flight is observable via selectors)", () => {
    const a = buildRaid(homeAttr, 1)
    const b = buildTackle(awayAttr)
    const s = reduce(EMPTY_QUEUE_STATE,
      { type: "ENQUEUE", built: a, now: 1 },
      { type: "ENQUEUE", built: b, now: 2 },
      { type: "MARK_SENDING", clientEventId: a.clientEventId },
    )
    expect(isSending(s)).toBe(true)
    // The orchestrator must pick the first pending only when none is sending.
    expect(firstPending(s)?.clientEventId).toBe(b.clientEventId)
  })

  it("4xx → failed_permanent is surfaced and still counts as unsynced", () => {
    const a = buildRaid(homeAttr, 1)
    let s = enqueue(EMPTY_QUEUE_STATE, a)
    s = reduce(s,
      { type: "MARK_SENDING", clientEventId: a.clientEventId },
      { type: "MARK_FAILED", clientEventId: a.clientEventId, errorMessage: "422" },
    )
    expect(s.actions[0].status).toBe("failed_permanent")
    expect(hasFailedPermanent(s)).toBe(true)
    expect(unsyncedCount(s)).toBe(1)
  })
})

// ── DUPLICATE SUBMISSION — the exactly-once proof ───────────────────────────

describe("reconcile — no duplicate submission", () => {
  it("a needs_reconcile action present on the server is CONFIRMED, never resent", () => {
    const raid = buildRaid(homeAttr, 3)
    let s = enqueue(EMPTY_QUEUE_STATE, raid)
    s = reduce(s,
      { type: "MARK_SENDING", clientEventId: raid.clientEventId },
      { type: "MARK_NEEDS_RECONCILE", clientEventId: raid.clientEventId, errorMessage: "timeout" },
    )
    // The POST actually landed (response was lost). Reconcile finds it on the log.
    const serverEvents = [serverEventFrom(raid, "S1", 1)]
    s = queueReducer(s, { type: "RECONCILE", confirmed: buildConfirmedMap(serverEvents) })

    expect(s.actions[0].status).toBe("confirmed")
    expect(s.actions[0].serverId).toBe("S1")
    expect(firstPending(s)).toBeUndefined() // nothing to resend → no duplicate
  })

  it("a needs_reconcile action ABSENT from the server becomes pending (safe to resend)", () => {
    const raid = buildRaid(homeAttr, 3)
    let s = enqueue(EMPTY_QUEUE_STATE, raid)
    s = reduce(s,
      { type: "MARK_SENDING", clientEventId: raid.clientEventId },
      { type: "MARK_NEEDS_RECONCILE", clientEventId: raid.clientEventId },
    )
    s = queueReducer(s, { type: "RECONCILE", confirmed: buildConfirmedMap([]) })
    expect(s.actions[0].status).toBe("pending")
    expect(firstPending(s)?.clientEventId).toBe(raid.clientEventId)
  })

  it("reconcile never disturbs an in-flight (sending) action that is absent", () => {
    const raid = buildRaid(homeAttr, 1)
    let s = enqueue(EMPTY_QUEUE_STATE, raid)
    s = queueReducer(s, { type: "MARK_SENDING", clientEventId: raid.clientEventId })
    s = queueReducer(s, { type: "RECONCILE", confirmed: buildConfirmedMap([]) })
    expect(s.actions[0].status).toBe("sending")
  })
})

// ── REFRESH / CRASH — ambiguous in-flight normalized ────────────────────────

describe("refresh / crash recovery", () => {
  it("NORMALIZE_ON_LOAD converts a persisted `sending` action to needs_reconcile", () => {
    const raid = buildRaid(homeAttr, 2)
    let s = enqueue(EMPTY_QUEUE_STATE, raid)
    s = queueReducer(s, { type: "MARK_SENDING", clientEventId: raid.clientEventId })
    // Simulate reload: the in-flight POST's fate is unknown.
    s = queueReducer(s, { type: "NORMALIZE_ON_LOAD" })
    expect(s.actions[0].status).toBe("needs_reconcile")
    // → it will be reconciled (not blindly resent) before any new send.
    expect(firstNeedsReconcile(s)?.clientEventId).toBe(raid.clientEventId)
  })

  it("persists and rehydrates the queue across a reload", () => {
    const raid = buildRaid(homeAttr, 2)
    const s = enqueue(EMPTY_QUEUE_STATE, raid)
    saveQueue("m-refresh", s)
    const loaded = loadQueue("m-refresh")
    expect(loaded?.actions).toHaveLength(1)
    expect(loaded?.actions[0].clientEventId).toBe(raid.clientEventId)
    clearQueue("m-refresh")
    expect(loadQueue("m-refresh")).toBeNull()
  })

  it("loadQueue returns null for corrupt storage", () => {
    window.localStorage.setItem("playarena.scorer.queue.m-bad", "{not json")
    expect(loadQueue("m-bad")).toBeNull()
    window.localStorage.setItem("playarena.scorer.queue.m-bad2", JSON.stringify({ actions: "x" }))
    expect(loadQueue("m-bad2")).toBeNull()
  })
})

// ── OFFLINE — nothing lost, optimistic reflects intent ──────────────────────

describe("offline behaviour", () => {
  it("actions queued offline stay pending and are reflected in the optimistic score", () => {
    let s = EMPTY_QUEUE_STATE
    s = enqueue(s, buildRaid(homeAttr, 3))
    s = enqueue(s, buildTackle(awayAttr))
    // Offline: nothing confirmed.
    expect(unsyncedCount(s)).toBe(2)
    expect(optimisticScore(MATCH, [], s.actions)).toEqual({ home: 3, away: 1 })
  })
})

// ── PRUNE / exactly-once across the confirm→refetch window ───────────────────

describe("optimistic score — exactly-once, no double counting", () => {
  it("an action counted on the server is NOT also counted as a pending pseudo-event", () => {
    const raid = buildRaid(homeAttr, 3)
    let s = enqueue(EMPTY_QUEUE_STATE, raid)
    s = reduce(s,
      { type: "MARK_SENDING", clientEventId: raid.clientEventId },
      { type: "MARK_CONFIRMED", clientEventId: raid.clientEventId, serverId: "S1", serverSequence: 1 },
    )
    const serverEvents = [serverEventFrom(raid, "S1", 1)]
    // Confirmed action still in queue AND present on server → counted ONCE.
    expect(optimisticScore(MATCH, serverEvents, s.actions)).toEqual({ home: 3, away: 0 })
  })

  it("PRUNE_CONFIRMED drops a confirmed action once it is on the server log", () => {
    const raid = buildRaid(homeAttr, 3)
    let s = enqueue(EMPTY_QUEUE_STATE, raid)
    s = reduce(s,
      { type: "MARK_SENDING", clientEventId: raid.clientEventId },
      { type: "MARK_CONFIRMED", clientEventId: raid.clientEventId, serverId: "S1", serverSequence: 1 },
    )
    const ids = serverClientIds([serverEventFrom(raid, "S1", 1)])
    s = queueReducer(s, { type: "PRUNE_CONFIRMED", serverClientIds: ids })
    expect(s.actions).toHaveLength(0)
  })

  it("failed_permanent actions never contribute to the score", () => {
    const raid = buildRaid(homeAttr, 5)
    let s = enqueue(EMPTY_QUEUE_STATE, raid)
    s = reduce(s,
      { type: "MARK_SENDING", clientEventId: raid.clientEventId },
      { type: "MARK_FAILED", clientEventId: raid.clientEventId, errorMessage: "422" },
    )
    expect(optimisticScore(MATCH, [], s.actions)).toEqual({ home: 0, away: 0 })
  })
})

// ── UNDO model ──────────────────────────────────────────────────────────────

describe("undo model", () => {
  it("undo of a pending (un-sent) action removes it locally", () => {
    const raid = buildRaid(homeAttr, 2)
    const s = enqueue(EMPTY_QUEUE_STATE, raid)
    const target = selectUndoTarget(s, new Set())
    expect(target).toMatchObject({ kind: "remove", clientEventId: raid.clientEventId })
    const after = queueReducer(s, { type: "REMOVE_LOCAL", clientEventId: raid.clientEventId })
    expect(after.actions).toHaveLength(0)
  })

  it("REMOVE_LOCAL refuses to remove a non-pending action", () => {
    const raid = buildRaid(homeAttr, 2)
    let s = enqueue(EMPTY_QUEUE_STATE, raid)
    s = queueReducer(s, { type: "MARK_SENDING", clientEventId: raid.clientEventId })
    const after = queueReducer(s, { type: "REMOVE_LOCAL", clientEventId: raid.clientEventId })
    expect(after.actions).toHaveLength(1) // still there — undo of in-flight is not a local remove
  })

  it("undo of a confirmed action targets a correction by server id", () => {
    const raid = buildRaid(homeAttr, 2)
    let s = enqueue(EMPTY_QUEUE_STATE, raid)
    s = reduce(s,
      { type: "MARK_SENDING", clientEventId: raid.clientEventId },
      { type: "MARK_CONFIRMED", clientEventId: raid.clientEventId, serverId: "S1", serverSequence: 1 },
    )
    expect(selectUndoTarget(s, new Set())).toMatchObject({ kind: "correct", serverId: "S1" })
  })

  it("an action already cancelled by a queued correction is not undoable again", () => {
    const raid = buildRaid(homeAttr, 2)
    let s = enqueue(EMPTY_QUEUE_STATE, raid)
    s = reduce(s,
      { type: "MARK_SENDING", clientEventId: raid.clientEventId },
      { type: "MARK_CONFIRMED", clientEventId: raid.clientEventId, serverId: "S1", serverSequence: 1 },
    )
    s = enqueue(s, buildCorrection("S1")) // correction queued
    expect(selectUndoTarget(s, new Set()).kind).toBe("none")
  })

  it("undo waits (busy) while the latest scoring action is in-flight", () => {
    const raid = buildRaid(homeAttr, 2)
    let s = enqueue(EMPTY_QUEUE_STATE, raid)
    s = queueReducer(s, { type: "MARK_SENDING", clientEventId: raid.clientEventId })
    expect(selectUndoTarget(s, new Set()).kind).toBe("busy")
  })

  it("optimistic score reflects a queued correction cancelling a confirmed raid", () => {
    const raid = buildRaid(homeAttr, 3)
    let s = enqueue(EMPTY_QUEUE_STATE, raid)
    s = reduce(s,
      { type: "MARK_SENDING", clientEventId: raid.clientEventId },
      { type: "MARK_CONFIRMED", clientEventId: raid.clientEventId, serverId: "S1", serverSequence: 1 },
    )
    const serverEvents = [serverEventFrom(raid, "S1", 1)]
    s = enqueue(s, buildCorrection("S1"))
    expect(optimisticScore(MATCH, serverEvents, s.actions)).toEqual({ home: 0, away: 0 })
  })
})

// ── server cancelled ids (timeline/undo) ────────────────────────────────────

describe("serverCancelledIds", () => {
  it("reports ids cancelled by a server-side correction", () => {
    const events: MatchEvent[] = [
      serverEventFrom(buildRaid(homeAttr, 2), "S1", 1),
      { ...serverEventFrom(buildCorrection("S1"), "S2", 2), cancels_event_id: "S1" },
    ]
    expect(serverCancelledIds(events)).toEqual(new Set(["S1"]))
  })
})

// ── completion gate ─────────────────────────────────────────────────────────

describe("completion gate", () => {
  const score = (h: number, a: number): LiveScore => ({
    match_id: "m", match_status: "live", home_score: h, away_score: a,
    home_team_id: HOME, away_team_id: AWAY, home_player_id: null, away_player_id: null, is_walkover: false,
  })

  it("blocks completion while events are unsynced", () => {
    const r = evaluateCompletion({ status: "live", unsyncedCount: 2, hasFailed: false, score: score(5, 3) })
    expect(r.ready).toBe(false)
    expect(r.reason).toMatch(/Sync 2 events/)
  })

  it("blocks completion when a non-live match", () => {
    const r = evaluateCompletion({ status: "scheduled", unsyncedCount: 0, hasFailed: false, score: score(0, 0) })
    expect(r.ready).toBe(false)
  })

  it("blocks completion when an event was permanently rejected", () => {
    const r = evaluateCompletion({ status: "live", unsyncedCount: 1, hasFailed: true, score: score(5, 3) })
    expect(r.ready).toBe(false)
    expect(r.reason).toMatch(/rejected events/)
  })

  it("blocks completion until the authoritative score is loaded", () => {
    const r = evaluateCompletion({ status: "live", unsyncedCount: 0, hasFailed: false, score: null })
    expect(r.ready).toBe(false)
  })

  it("is ready when fully synced and derives the winner from the authoritative score", () => {
    expect(evaluateCompletion({ status: "live", unsyncedCount: 0, hasFailed: false, score: score(8, 5) }).winner)
      .toEqual({ side: "home", participantId: HOME })
    expect(evaluateCompletion({ status: "live", unsyncedCount: 0, hasFailed: false, score: score(5, 8) }).winner)
      .toEqual({ side: "away", participantId: AWAY })
    expect(evaluateCompletion({ status: "live", unsyncedCount: 0, hasFailed: false, score: score(7, 7) }).winner)
      .toEqual({ side: "draw" })
  })
})

// ── localStorage isolation ──────────────────────────────────────────────────

beforeEach(() => {
  window.localStorage.clear()
})
