"use client"

import { useMemo } from "react"
import { computeScore, type ScoringMatch, type ComputedScore } from "@/lib/scoring/engine"
import type { MatchEvent } from "@/types/api/match-events"

/**
 * Deterministic client-side fold of the event log into a score, using the
 * scoring engine that mirrors the backend. In FE-7BA this is read-only and used
 * to cross-check the authoritative GET /score; in FE-7BB it backs optimistic
 * display. Returns null until both inputs are present so callers can prefer the
 * authoritative score while data loads.
 */
export function useLocalScore(
  match: ScoringMatch | null | undefined,
  events: MatchEvent[] | null | undefined,
): ComputedScore | null {
  return useMemo(() => {
    if (!match || !events) return null
    return computeScore(match, events)
  }, [match, events])
}
