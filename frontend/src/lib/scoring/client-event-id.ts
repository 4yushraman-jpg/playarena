/**
 * Generates a unique client_event_id for every scoring action. This id is
 * embedded in the event payload and is the linchpin of the exactly-once
 * guarantee: the backend has no idempotency of its own, so the client uses this
 * id + the server event log as a deduplication oracle (see reconcile.ts).
 *
 * A v4 UUID from the platform crypto is used when available, with a sufficiently
 * unique fallback for older runtimes. Uniqueness only needs to hold within one
 * match's event stream.
 */
export function newClientEventId(): string {
  const c = (globalThis as { crypto?: Crypto }).crypto
  if (c && typeof c.randomUUID === "function") {
    return c.randomUUID()
  }
  // Fallback: time + randomness. Not RFC-4122 strict, but collision-safe within
  // a single match.
  const rand = () => Math.random().toString(16).slice(2, 10)
  return `cev-${Date.now().toString(16)}-${rand()}-${rand()}`
}
