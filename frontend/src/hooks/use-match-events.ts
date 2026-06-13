"use client"

import { useQuery } from "@tanstack/react-query"
import { matchEventsApi } from "@/lib/api/match-events"
import { matchKeys } from "@/lib/query-keys"
import type { MatchEventListParams } from "@/types/api/match-events"

// Fetch the full event history (not the netted view) so the timeline can show
// corrections and the local engine can compute the effective score itself.
// A generous page bounds a single read; full pagination of very long matches
// is a FE-7BB concern (incremental scoring). The authoritative headline score
// always comes from GET /score regardless, so this never drives the result.
const FULL_HISTORY_PARAMS: MatchEventListParams = { effective_only: false, limit: 500, offset: 0 }

export function useMatchEvents(orgSlug: string, matchId: string, enabled = true) {
  return useQuery({
    queryKey: matchKeys.events(orgSlug, matchId, FULL_HISTORY_PARAMS),
    queryFn: () =>
      matchEventsApi.list(orgSlug, matchId, FULL_HISTORY_PARAMS).then((r) => r.data),
    staleTime: 10_000,
    enabled: enabled && !!orgSlug && !!matchId,
  })
}
