import type { CreateMatchEventRequest } from "@/types/api/match-events"

/**
 * Lifecycle of a queued scoring action:
 *
 *   pending          enqueued, not yet attempted (or reset after reconcile)
 *   sending          exactly one in-flight POST (single-flight invariant)
 *   needs_reconcile  a transport error occurred — landing is UNKNOWN, so it
 *                    must be reconciled against the server log before any resend
 *   confirmed        server accepted it (serverId known); on the server log
 *   failed_permanent a 4xx validation rejection — surfaced, never auto-retried
 *
 * INVARIANTS:
 *  - At most one action is `sending` at any time.
 *  - An action is only ever resent after reconcile proves it is NOT on the
 *    server. This is what makes delivery exactly-once.
 *  - `confirmed` is terminal-success; `failed_permanent` is terminal-failure.
 */
export type ActionStatus =
  | "pending"
  | "sending"
  | "needs_reconcile"
  | "confirmed"
  | "failed_permanent"

export interface QueuedAction {
  clientEventId: string
  localSeq: number
  body: CreateMatchEventRequest
  /** Point-bearing action (eligible for undo). Corrections are false. */
  isScoring: boolean
  /** For corrections: the server id of the cancelled event. */
  cancelsServerId?: string
  status: ActionStatus
  serverId?: string
  serverSequence?: number
  attemptCount: number
  errorMessage?: string
  createdAt: number
}

export interface QueueState {
  actions: QueuedAction[]
  nextLocalSeq: number
}

export const EMPTY_QUEUE_STATE: QueueState = { actions: [], nextLocalSeq: 1 }
