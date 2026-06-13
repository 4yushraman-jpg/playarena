import type { QueueState } from "./queue-types"

// localStorage persistence so the queue survives refresh, crash, and device
// sleep. Keyed per match. All access is defensive — storage may be unavailable
// (SSR, private mode, quota) and must never throw into the scorer.

const PREFIX = "playarena.scorer.queue."
const storageKey = (matchId: string) => `${PREFIX}${matchId}`

export function loadQueue(matchId: string): QueueState | null {
  if (typeof window === "undefined") return null
  try {
    const raw = window.localStorage.getItem(storageKey(matchId))
    if (!raw) return null
    const parsed: unknown = JSON.parse(raw)
    if (
      !parsed ||
      typeof parsed !== "object" ||
      !Array.isArray((parsed as QueueState).actions) ||
      typeof (parsed as QueueState).nextLocalSeq !== "number"
    ) {
      return null
    }
    return parsed as QueueState
  } catch {
    return null
  }
}

export function saveQueue(matchId: string, state: QueueState): void {
  if (typeof window === "undefined") return
  try {
    window.localStorage.setItem(storageKey(matchId), JSON.stringify(state))
  } catch {
    // Quota/private-mode failures must not break scoring; the in-memory queue
    // remains authoritative for the session.
  }
}

export function clearQueue(matchId: string): void {
  if (typeof window === "undefined") return
  try {
    window.localStorage.removeItem(storageKey(matchId))
  } catch {
    // ignore
  }
}
