import { describe, it, expect, vi, beforeEach, afterEach } from "vitest"
import { renderHook, act } from "@testing-library/react"
import { useScoringQueue } from "@/hooks/use-scoring-queue"
import { buildBonus, buildTackle, type BuiltAction } from "@/lib/scoring/scoring-actions"
import type { MatchEvent, CreateMatchEventRequest } from "@/types/api/match-events"

const homeAttr = { teamMode: true, participantId: "home-team" }
const awayAttr = { teamMode: true, participantId: "away-team" }

// Build a server event echoing a queued action (carries its client_event_id).
function serverEventFor(body: CreateMatchEventRequest, id: string, seq: number): MatchEvent {
  return {
    id, match_id: "m", organization_id: "o", sequence_number: seq,
    event_type: body.event_type, team_id: body.team_id ?? null, player_id: body.player_id ?? null,
    period: body.period ?? 1, clock_seconds: null,
    payload: (body.payload ?? {}) as Record<string, unknown>,
    recorded_by: "u", recorded_at: "2026-07-01T10:00:00Z",
    cancels_event_id: body.cancels_event_id ?? null, created_at: "2026-07-01T10:00:00Z",
  }
}

function netError() {
  return { isAxiosError: true, message: "network down" }
}
function validationError() {
  return { isAxiosError: true, response: { status: 422, data: { error: "invalid" } } }
}

function setOnline(value: boolean) {
  Object.defineProperty(navigator, "onLine", { configurable: true, value })
}

// Stable references — the hook documents that its I/O callbacks and
// serverEvents must be referentially stable (the container memoizes them).
const STABLE_EMPTY_EVENTS: MatchEvent[] = []
const NOOP = () => {}

interface HarnessArgs {
  postEvent: (b: CreateMatchEventRequest) => Promise<MatchEvent>
  fetchServerEvents: () => Promise<MatchEvent[]>
}
function render({ postEvent, fetchServerEvents }: HarnessArgs) {
  return renderHook(() =>
    useScoringQueue({
      matchId: "m-queue",
      serverEvents: STABLE_EMPTY_EVENTS,
      postEvent,
      fetchServerEvents,
      onServerChanged: NOOP,
    }),
  )
}

async function flush() {
  // Flush the pump chain: microtasks + any backoff timers.
  await act(async () => {
    await vi.advanceTimersByTimeAsync(20_000)
  })
}

beforeEach(() => {
  vi.useFakeTimers()
  window.localStorage.clear()
  setOnline(true)
})
afterEach(() => {
  vi.useRealTimers()
})

// ── Happy path ───────────────────────────────────────────────────────────────

describe("useScoringQueue — happy path", () => {
  it("sends one pending action exactly once and confirms it", async () => {
    const action = buildBonus(homeAttr)
    const postEvent = vi.fn().mockResolvedValue(serverEventFor(action.body, "S1", 1))
    const fetchServerEvents = vi.fn().mockResolvedValue([])
    const { result } = render({ postEvent, fetchServerEvents })

    await act(async () => {
      result.current.enqueue(action)
    })
    await flush()

    expect(postEvent).toHaveBeenCalledTimes(1)
    expect(result.current.unsyncedCount).toBe(0)
  })
})

// ── OFFLINE ────────────────────────────────────────────────────────────────

describe("useScoringQueue — offline", () => {
  it("queues offline without sending, then flushes on reconnect", async () => {
    setOnline(false)
    const action = buildBonus(homeAttr)
    const postEvent = vi.fn().mockResolvedValue(serverEventFor(action.body, "S1", 1))
    const fetchServerEvents = vi.fn().mockResolvedValue([])
    const { result } = render({ postEvent, fetchServerEvents })

    await act(async () => {
      result.current.enqueue(action)
    })
    await flush()
    expect(postEvent).not.toHaveBeenCalled()
    expect(result.current.unsyncedCount).toBe(1)

    // Reconnect.
    await act(async () => {
      setOnline(true)
      window.dispatchEvent(new Event("online"))
    })
    await flush()
    expect(postEvent).toHaveBeenCalledTimes(1)
    expect(result.current.unsyncedCount).toBe(0)
  })
})

// ── DUPLICATE / lost response — exactly-once ─────────────────────────────────

describe("useScoringQueue — lost response (no duplicate)", () => {
  it("does NOT resend when the action actually landed (reconcile confirms it)", async () => {
    const action = buildBonus(homeAttr)
    // POST rejects (network) but the event DID land — fetch returns it.
    const postEvent = vi.fn().mockRejectedValue(netError())
    const fetchServerEvents = vi.fn().mockResolvedValue([serverEventFor(action.body, "S1", 1)])
    const { result } = render({ postEvent, fetchServerEvents })

    await act(async () => {
      result.current.enqueue(action)
    })
    await flush()

    expect(postEvent).toHaveBeenCalledTimes(1) // never resent → no duplicate
    expect(fetchServerEvents).toHaveBeenCalled() // reconciled before any resend
    expect(result.current.unsyncedCount).toBe(0) // confirmed via the server log
  })
})

// ── RECONNECT resend — action truly absent ───────────────────────────────────

describe("useScoringQueue — transient failure then success", () => {
  it("reconciles (absent) then resends until confirmed", async () => {
    const action = buildBonus(homeAttr)
    const postEvent = vi
      .fn()
      .mockRejectedValueOnce(netError()) // first attempt: network error
      .mockResolvedValueOnce(serverEventFor(action.body, "S1", 1)) // resend succeeds
    const fetchServerEvents = vi.fn().mockResolvedValue([]) // proven NOT landed
    const { result } = render({ postEvent, fetchServerEvents })

    await act(async () => {
      result.current.enqueue(action)
    })
    await flush()

    expect(fetchServerEvents).toHaveBeenCalled()
    expect(postEvent).toHaveBeenCalledTimes(2)
    expect(result.current.unsyncedCount).toBe(0)
  })
})

// ── SINGLE-FLIGHT ────────────────────────────────────────────────────────────

describe("useScoringQueue — single-flight FIFO", () => {
  it("sends actions one at a time, in order", async () => {
    const a = buildBonus(homeAttr)
    const b = buildTackle(awayAttr)
    const order: string[] = []
    let inFlight = 0
    let maxConcurrent = 0
    const postEvent = vi.fn(async (body: CreateMatchEventRequest) => {
      inFlight += 1
      maxConcurrent = Math.max(maxConcurrent, inFlight)
      order.push(body.event_type)
      await Promise.resolve()
      inFlight -= 1
      return serverEventFor(body, `S-${order.length}`, order.length)
    })
    const fetchServerEvents = vi.fn().mockResolvedValue([])
    const { result } = render({ postEvent, fetchServerEvents })

    await act(async () => {
      result.current.enqueue(a)
      result.current.enqueue(b)
    })
    await flush()

    expect(maxConcurrent).toBe(1) // never two in flight
    expect(order).toEqual(["bonus_point_awarded", "tackle_successful"]) // FIFO
    expect(result.current.unsyncedCount).toBe(0)
  })
})

// ── PERMANENT (4xx) failure ──────────────────────────────────────────────────

describe("useScoringQueue — permanent failure", () => {
  it("marks a 4xx-rejected action failed_permanent and never retries it", async () => {
    const action = buildBonus(homeAttr)
    const postEvent = vi.fn().mockRejectedValue(validationError())
    const fetchServerEvents = vi.fn().mockResolvedValue([])
    const { result } = render({ postEvent, fetchServerEvents })

    await act(async () => {
      result.current.enqueue(action)
    })
    await flush()

    expect(postEvent).toHaveBeenCalledTimes(1) // not retried
    expect(result.current.hasFailed).toBe(true)
    expect(result.current.unsyncedCount).toBe(1)
  })
})

// ── REFRESH / crash recovery ─────────────────────────────────────────────────

describe("useScoringQueue — refresh recovery", () => {
  it("normalizes a persisted in-flight action and reconciles it instead of blind resend", async () => {
    // Simulate a crash mid-send: an action persisted as `sending`.
    const action: BuiltAction = buildBonus(homeAttr)
    const persisted = {
      actions: [
        {
          clientEventId: action.clientEventId,
          localSeq: 1,
          body: action.body,
          isScoring: true,
          status: "sending",
          attemptCount: 1,
          createdAt: Date.now(),
        },
      ],
      nextLocalSeq: 2,
    }
    window.localStorage.setItem("playarena.scorer.queue.m-queue", JSON.stringify(persisted))

    // It had actually landed before the crash.
    const postEvent = vi.fn()
    const fetchServerEvents = vi.fn().mockResolvedValue([serverEventFor(action.body, "S1", 1)])
    const { result } = render({ postEvent, fetchServerEvents })

    await flush()

    expect(fetchServerEvents).toHaveBeenCalled() // reconciled, not blindly resent
    expect(postEvent).not.toHaveBeenCalled() // no duplicate
    expect(result.current.unsyncedCount).toBe(0) // confirmed from the log
  })
})

// ── UNDO ─────────────────────────────────────────────────────────────────────

describe("useScoringQueue — undo", () => {
  it("undo of a still-pending (offline) action removes it without ever sending", async () => {
    setOnline(false)
    const action = buildBonus(homeAttr)
    const postEvent = vi.fn()
    const fetchServerEvents = vi.fn().mockResolvedValue([])
    const { result } = render({ postEvent, fetchServerEvents })

    await act(async () => {
      result.current.enqueue(action)
    })
    expect(result.current.unsyncedCount).toBe(1)

    await act(async () => {
      result.current.undo()
    })
    expect(result.current.unsyncedCount).toBe(0)

    setOnline(true)
    await act(async () => {
      window.dispatchEvent(new Event("online"))
    })
    await flush()
    expect(postEvent).not.toHaveBeenCalled() // nothing to send — it was removed
  })
})
