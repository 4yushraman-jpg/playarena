"use client"

import { useCallback, useEffect, useReducer, useRef, useState } from "react"
import { isAxiosError } from "axios"
import {
  queueReducer,
  firstPending,
  firstNeedsReconcile,
  isSending,
  unsyncedCount as selectUnsyncedCount,
  hasFailedPermanent,
  selectUndoTarget,
} from "@/lib/scoring/queue-reducer"
import { EMPTY_QUEUE_STATE, type QueueState, type QueuedAction } from "@/lib/scoring/queue-types"
import { buildCorrection, type BuiltAction } from "@/lib/scoring/scoring-actions"
import { buildConfirmedMap, serverClientIds, serverCancelledIds } from "@/lib/scoring/reconcile"
import { loadQueue, saveQueue } from "@/lib/scoring/queue-storage"
import type { MatchEvent, CreateMatchEventRequest } from "@/types/api/match-events"

const MAX_BACKOFF_MS = 15_000
const BASE_BACKOFF_MS = 1_000

interface UseScoringQueueArgs {
  matchId: string
  serverEvents: MatchEvent[]
  postEvent: (body: CreateMatchEventRequest) => Promise<MatchEvent>
  fetchServerEvents: () => Promise<MatchEvent[]>
  /** Called after the server log changes (confirm/reconcile) so the caller can
   *  refetch the events/score queries. Must be stable. */
  onServerChanged?: () => void
}

export interface ScoringQueueApi {
  actions: QueuedAction[]
  enqueue: (built: BuiltAction) => void
  undo: () => void
  undoTarget: ReturnType<typeof selectUndoTarget>
  unsyncedCount: number
  hasFailed: boolean
  isOnline: boolean
  isSyncing: boolean
}

/**
 * Orchestrates the exactly-once scoring queue: persistence, single-flight FIFO
 * sending, reconcile-before-resend, backoff, and reconnect/visibility recovery.
 *
 * The integrity logic lives in the pure reducer/reconcile modules. This hook is
 * driven by the *actionable head* of the queue: a single effect performs one
 * async step (reconcile or send) per discrete change to that head, guarded by
 * `inFlightRef` so there is never more than one in-flight request. Invariants:
 *   - at most one in-flight POST,
 *   - a needs_reconcile action is reconciled (GET log) before any resend,
 *   - a persisted `sending` action is normalized to needs_reconcile on load.
 *
 * `postEvent`, `fetchServerEvents` and `onServerChanged` MUST be referentially
 * stable (wrap in useCallback) so the driver effect does not thrash.
 */
export function useScoringQueue({
  matchId,
  serverEvents,
  postEvent,
  fetchServerEvents,
  onServerChanged,
}: UseScoringQueueArgs): ScoringQueueApi {
  const [state, dispatch] = useReducer(
    queueReducer,
    matchId,
    (id): QueueState => {
      const loaded = loadQueue(id)
      // A persisted in-flight action is ambiguous after reload/crash → treat as
      // needs_reconcile so it is never blindly resent.
      return loaded ? queueReducer(loaded, { type: "NORMALIZE_ON_LOAD" }) : EMPTY_QUEUE_STATE
    },
  )

  const [isOnline, setIsOnline] = useState(() =>
    typeof navigator === "undefined" ? true : navigator.onLine,
  )
  // Bumped to re-trigger the driver after a backoff delay or on reconnect/visibility.
  const [wake, setWake] = useState(0)

  const inFlightRef = useRef(false)
  const backoffTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const attemptRef = useRef(0)

  // Persist on every change so the queue survives refresh / crash / sleep.
  useEffect(() => {
    saveQueue(matchId, state)
  }, [matchId, state])

  // Keep the queue tidy: drop confirmed actions once the display event log
  // already includes them (counted from the server side now).
  useEffect(() => {
    const ids = serverClientIds(serverEvents)
    if (ids.size > 0) dispatch({ type: "PRUNE_CONFIRMED", serverClientIds: ids })
  }, [serverEvents])

  const scheduleBackoff = useCallback(() => {
    if (backoffTimerRef.current) clearTimeout(backoffTimerRef.current)
    const attempt = (attemptRef.current += 1)
    const delay = Math.min(BASE_BACKOFF_MS * 2 ** (attempt - 1), MAX_BACKOFF_MS)
    backoffTimerRef.current = setTimeout(() => {
      backoffTimerRef.current = null
      setWake((w) => w + 1)
    }, delay)
  }, [])

  // The actionable head: a needs_reconcile action must be resolved before any
  // pending send. Encode it as a key so the driver re-runs only on real change.
  const head: QueuedAction | undefined = firstNeedsReconcile(state) ?? firstPending(state)
  const sending = isSending(state)
  const headKey = head ? `${head.status}:${head.clientEventId}:${head.attemptCount}` : "idle"

  useEffect(() => {
    // Single-flight: never start a step while one is in flight or an action is
    // mid-send. The effect re-runs (via `sending`/headKey deps) once that clears.
    if (inFlightRef.current || sending || !isOnline || !head) return

    inFlightRef.current = true
    void (async () => {
      try {
        if (head.status === "needs_reconcile") {
          // Reconcile BEFORE any resend — the server log is the dedup oracle.
          const events = await fetchServerEvents()
          dispatch({ type: "RECONCILE", confirmed: buildConfirmedMap(events) })
          dispatch({ type: "PRUNE_CONFIRMED", serverClientIds: serverClientIds(events) })
          onServerChanged?.()
          attemptRef.current = 0
        } else {
          // Persist the in-flight intent first (so a crash mid-send is recovered
          // as needs_reconcile, never blindly resent), then POST.
          dispatch({ type: "MARK_SENDING", clientEventId: head.clientEventId })
          const created = await postEvent(head.body)
          dispatch({
            type: "MARK_CONFIRMED",
            clientEventId: head.clientEventId,
            serverId: created.id,
            serverSequence: created.sequence_number,
          })
          onServerChanged?.()
          attemptRef.current = 0
        }
      } catch (err) {
        if (head.status === "pending" && isPermanentError(err)) {
          dispatch({
            type: "MARK_FAILED",
            clientEventId: head.clientEventId,
            errorMessage: errorMessageOf(err),
          })
        } else {
          // Unknown outcome (network / 5xx): mark for reconcile + back off.
          dispatch({
            type: "MARK_NEEDS_RECONCILE",
            clientEventId: head.clientEventId,
            errorMessage: errorMessageOf(err),
          })
          scheduleBackoff()
        }
      } finally {
        inFlightRef.current = false
      }
    })()
    // eslint-disable-next-line react-hooks/exhaustive-deps -- headKey/sending encode `head`; fns are stable
  }, [headKey, sending, isOnline, wake, fetchServerEvents, postEvent, onServerChanged, scheduleBackoff])

  // Connectivity + visibility recovery.
  useEffect(() => {
    const onOnline = () => {
      setIsOnline(true)
      setWake((w) => w + 1)
    }
    const onOffline = () => setIsOnline(false)
    const onVisible = () => {
      if (typeof document !== "undefined" && document.visibilityState === "visible") {
        setWake((w) => w + 1)
      }
    }
    window.addEventListener("online", onOnline)
    window.addEventListener("offline", onOffline)
    document.addEventListener("visibilitychange", onVisible)
    return () => {
      window.removeEventListener("online", onOnline)
      window.removeEventListener("offline", onOffline)
      document.removeEventListener("visibilitychange", onVisible)
      if (backoffTimerRef.current) clearTimeout(backoffTimerRef.current)
    }
  }, [])

  const enqueue = useCallback((built: BuiltAction) => {
    dispatch({ type: "ENQUEUE", built, now: Date.now() })
  }, [])

  const undoTarget = selectUndoTarget(state, serverCancelledIds(serverEvents))

  const undo = useCallback(() => {
    if (undoTarget.kind === "remove") {
      dispatch({ type: "REMOVE_LOCAL", clientEventId: undoTarget.clientEventId })
    } else if (undoTarget.kind === "correct") {
      dispatch({ type: "ENQUEUE", built: buildCorrection(undoTarget.serverId), now: Date.now() })
    }
    // busy / none → no-op
  }, [undoTarget])

  return {
    actions: state.actions,
    enqueue,
    undo,
    undoTarget,
    unsyncedCount: selectUnsyncedCount(state),
    hasFailed: hasFailedPermanent(state),
    isOnline,
    isSyncing: sending || firstNeedsReconcile(state) != null,
  }
}

// ── error classification ───────────────────────────────────────────────────

// A 4xx response means the write will never succeed on retry (validation,
// permission, match-not-live) → permanent. No response (network) or 5xx →
// transient: outcome unknown, must reconcile before resend.
function isPermanentError(err: unknown): boolean {
  if (isAxiosError(err)) {
    const status = err.response?.status
    if (status == null) return false
    return status >= 400 && status < 500
  }
  return false
}

function errorMessageOf(err: unknown): string {
  if (isAxiosError(err)) {
    const data = err.response?.data as { error?: string } | undefined
    return data?.error ?? err.message
  }
  return err instanceof Error ? err.message : "Unknown error"
}
