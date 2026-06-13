"use client"

import { useQuery } from "@tanstack/react-query"
import { matchesApi } from "@/lib/api/matches"
import { matchKeys } from "@/lib/query-keys"

/**
 * Authoritative match score from `GET /matches/{id}/score`. This is the
 * source-of-truth number for display in the read-only scorer — derived
 * server-side from the effective event log. Read-only: no polling, no
 * mutations (those arrive in FE-7BB/FE-7C).
 */
export function useMatchScore(orgSlug: string, matchId: string, enabled = true) {
  return useQuery({
    queryKey: matchKeys.score(orgSlug, matchId),
    queryFn: () => matchesApi.getScore(orgSlug, matchId).then((r) => r.data),
    staleTime: 10_000,
    enabled: enabled && !!orgSlug && !!matchId,
  })
}
