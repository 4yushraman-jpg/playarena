import type { MatchEvent } from "@/types/api/match-events"
import { computeScore, type ComputedScore, type ScoringMatch } from "./engine"
import { serverClientIds } from "./reconcile"
import type { QueuedAction } from "./queue-types"

const PSEUDO_SEQ_BASE = 1_000_000_000

/**
 * Projects a queued action into a synthetic MatchEvent so the scoring engine
 * can fold it alongside real server events. Uses the server id once known so a
 * pending correction can reference it.
 */
export function actionToPseudoEvent(a: QueuedAction): MatchEvent {
  const b = a.body
  return {
    id: a.serverId ?? a.clientEventId,
    match_id: "",
    organization_id: "",
    sequence_number: PSEUDO_SEQ_BASE + a.localSeq,
    event_type: b.event_type,
    team_id: b.team_id ?? null,
    player_id: b.player_id ?? null,
    period: b.period ?? null,
    clock_seconds: b.clock_seconds ?? null,
    payload: (b.payload ?? {}) as Record<string, unknown>,
    recorded_by: null,
    recorded_at: new Date(a.createdAt).toISOString(),
    cancels_event_id: b.cancels_event_id ?? null,
    created_at: new Date(a.createdAt).toISOString(),
  }
}

/**
 * The event set used for optimistic display: server events PLUS queued actions
 * not yet on the server — deduplicated by client_event_id so an action counted
 * once on the server is never also counted as a pending pseudo-event. This is
 * what makes the optimistic score exactly-once across the confirm→refetch
 * window. failed_permanent actions never count.
 */
export function optimisticEvents(
  serverEvents: MatchEvent[],
  actions: QueuedAction[],
): MatchEvent[] {
  const onServer = serverClientIds(serverEvents)
  const pseudo = actions
    .filter((a) => a.status !== "failed_permanent")
    .filter((a) => !onServer.has(a.clientEventId))
    .map(actionToPseudoEvent)
  return [...serverEvents, ...pseudo]
}

/** Optimistic score = engine fold over (server events ∪ un-landed queued actions). */
export function optimisticScore(
  match: ScoringMatch,
  serverEvents: MatchEvent[],
  actions: QueuedAction[],
): ComputedScore {
  return computeScore(match, optimisticEvents(serverEvents, actions))
}
