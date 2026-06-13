import type { MatchEvent } from "@/types/api/match-events"
import { cancelledEventIds } from "./engine"

/**
 * Reconciliation primitives — the server event log is the deduplication oracle.
 * Every queued action embeds its client_event_id in the event payload, so the
 * presence of that id in the server log proves the action landed. Pure.
 */

function clientEventIdOf(e: MatchEvent): string | null {
  const cid = e.payload?.client_event_id
  return typeof cid === "string" ? cid : null
}

/** Maps client_event_id → server identity for every event already on the log. */
export function buildConfirmedMap(
  serverEvents: MatchEvent[],
): Map<string, { serverId: string; serverSequence: number }> {
  const map = new Map<string, { serverId: string; serverSequence: number }>()
  for (const e of serverEvents) {
    const cid = clientEventIdOf(e)
    if (cid) map.set(cid, { serverId: e.id, serverSequence: e.sequence_number })
  }
  return map
}

/** Set of client_event_ids already present on the server log. */
export function serverClientIds(serverEvents: MatchEvent[]): Set<string> {
  const s = new Set<string>()
  for (const e of serverEvents) {
    const cid = clientEventIdOf(e)
    if (cid) s.add(cid)
  }
  return s
}

/** Server-side event ids that have been cancelled by a correction. */
export function serverCancelledIds(serverEvents: MatchEvent[]): Set<string> {
  return cancelledEventIds(serverEvents)
}
