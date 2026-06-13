import type { BuiltAction } from "./scoring-actions"
import type { ActionStatus, QueueState, QueuedAction } from "./queue-types"

/**
 * Pure reducer for the scoring queue. All exactly-once integrity lives here and
 * in reconcile.ts — both are side-effect-free and exhaustively tested. The
 * orchestrator hook only wires this to I/O (POST, GET, localStorage, timers).
 */
export type QueueEvent =
  | { type: "HYDRATE"; state: QueueState }
  | { type: "NORMALIZE_ON_LOAD" }
  | { type: "ENQUEUE"; built: BuiltAction; now: number }
  | { type: "MARK_SENDING"; clientEventId: string }
  | { type: "MARK_CONFIRMED"; clientEventId: string; serverId: string; serverSequence: number }
  | { type: "MARK_NEEDS_RECONCILE"; clientEventId: string; errorMessage?: string }
  | { type: "MARK_FAILED"; clientEventId: string; errorMessage: string }
  | { type: "RECONCILE"; confirmed: Map<string, { serverId: string; serverSequence: number }> }
  | { type: "REMOVE_LOCAL"; clientEventId: string }
  | { type: "PRUNE_CONFIRMED"; serverClientIds: Set<string> }

function mapAction(
  state: QueueState,
  clientEventId: string,
  fn: (a: QueuedAction) => QueuedAction,
): QueueState {
  return {
    ...state,
    actions: state.actions.map((a) => (a.clientEventId === clientEventId ? fn(a) : a)),
  }
}

export function queueReducer(state: QueueState, event: QueueEvent): QueueState {
  switch (event.type) {
    case "HYDRATE":
      return event.state

    case "NORMALIZE_ON_LOAD":
      // A persisted `sending` action is ambiguous after a reload/crash: the POST
      // may or may not have landed. Treat it as needs_reconcile so it is never
      // blindly resent.
      return {
        ...state,
        actions: state.actions.map((a) =>
          a.status === "sending" ? { ...a, status: "needs_reconcile" as ActionStatus } : a,
        ),
      }

    case "ENQUEUE": {
      const action: QueuedAction = {
        clientEventId: event.built.clientEventId,
        localSeq: state.nextLocalSeq,
        body: event.built.body,
        isScoring: event.built.isScoring,
        cancelsServerId: event.built.cancelsServerId,
        status: "pending",
        attemptCount: 0,
        createdAt: event.now,
      }
      return {
        actions: [...state.actions, action],
        nextLocalSeq: state.nextLocalSeq + 1,
      }
    }

    case "MARK_SENDING":
      return mapAction(state, event.clientEventId, (a) => ({
        ...a,
        status: "sending",
        attemptCount: a.attemptCount + 1,
      }))

    case "MARK_CONFIRMED":
      return mapAction(state, event.clientEventId, (a) => ({
        ...a,
        status: "confirmed",
        serverId: event.serverId,
        serverSequence: event.serverSequence,
        errorMessage: undefined,
      }))

    case "MARK_NEEDS_RECONCILE":
      return mapAction(state, event.clientEventId, (a) => ({
        ...a,
        status: "needs_reconcile",
        errorMessage: event.errorMessage,
      }))

    case "MARK_FAILED":
      return mapAction(state, event.clientEventId, (a) => ({
        ...a,
        status: "failed_permanent",
        errorMessage: event.errorMessage,
      }))

    case "RECONCILE":
      // The server log is the dedup oracle. Any non-terminal action present on
      // the server is confirmed (adopt its server id). A needs_reconcile action
      // proven ABSENT becomes pending again — only now is it safe to resend.
      return {
        ...state,
        actions: state.actions.map((a) => {
          if (a.status === "confirmed" || a.status === "failed_permanent") return a
          const hit = event.confirmed.get(a.clientEventId)
          if (hit) {
            return {
              ...a,
              status: "confirmed" as ActionStatus,
              serverId: hit.serverId,
              serverSequence: hit.serverSequence,
              errorMessage: undefined,
            }
          }
          if (a.status === "needs_reconcile") {
            return { ...a, status: "pending" as ActionStatus }
          }
          return a
        }),
      }

    case "REMOVE_LOCAL":
      // Local undo of an action that never reached the server. Guarded: only a
      // `pending` action may be removed — never one that is in-flight or
      // already confirmed (those are undone via a correction).
      return {
        ...state,
        actions: state.actions.filter(
          (a) => !(a.clientEventId === event.clientEventId && a.status === "pending"),
        ),
      }

    case "PRUNE_CONFIRMED":
      // Drop confirmed actions once the server event log includes them (counted
      // from the server side now). Keeps the queue small without ever losing an
      // un-counted action.
      return {
        ...state,
        actions: state.actions.filter(
          (a) => !(a.status === "confirmed" && a.serverId && event.serverClientIds.has(a.clientEventId)),
        ),
      }

    default:
      return state
  }
}

// ── Selectors (pure) ─────────────────────────────────────────────────────────

/** Actions not yet known-on-server (blocks completion). Confirmed = synced. */
export function unsyncedActions(state: QueueState): QueuedAction[] {
  return state.actions.filter((a) => a.status !== "confirmed")
}

export function unsyncedCount(state: QueueState): number {
  return unsyncedActions(state).length
}

export function hasFailedPermanent(state: QueueState): boolean {
  return state.actions.some((a) => a.status === "failed_permanent")
}

export function isSending(state: QueueState): boolean {
  return state.actions.some((a) => a.status === "sending")
}

export function firstNeedsReconcile(state: QueueState): QueuedAction | undefined {
  return state.actions.find((a) => a.status === "needs_reconcile")
}

/** Next action eligible to send (FIFO by insertion/localSeq order). */
export function firstPending(state: QueueState): QueuedAction | undefined {
  return state.actions.find((a) => a.status === "pending")
}

export type UndoTarget =
  | { kind: "none" }
  | { kind: "busy" }
  | { kind: "remove"; clientEventId: string; describe: QueuedAction }
  | { kind: "correct"; serverId: string; describe: QueuedAction }

/**
 * Resolves what "undo" should do to the most recent still-counting scoring
 * action. `serverCancelledIds` are server ids already cancelled on the server
 * log; queued corrections are also accounted for so an action can't be undone
 * twice.
 */
export function selectUndoTarget(
  state: QueueState,
  serverCancelledIds: Set<string>,
): UndoTarget {
  const cancelled = new Set(serverCancelledIds)
  for (const a of state.actions) {
    if (a.cancelsServerId) cancelled.add(a.cancelsServerId)
  }

  const active = state.actions.filter(
    (a) =>
      a.isScoring &&
      a.status !== "failed_permanent" &&
      !(a.serverId && cancelled.has(a.serverId)),
  )
  if (active.length === 0) return { kind: "none" }

  const last = active.reduce((m, a) => (a.localSeq > m.localSeq ? a : m), active[0])

  if (last.status === "pending") {
    return { kind: "remove", clientEventId: last.clientEventId, describe: last }
  }
  if (last.status === "confirmed" && last.serverId) {
    return { kind: "correct", serverId: last.serverId, describe: last }
  }
  // sending / needs_reconcile: the latest action is in-flight or unresolved —
  // undo must wait so we never undo the wrong (older) action out of order.
  return { kind: "busy" }
}
