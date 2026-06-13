"use client"

import { useMutation, useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"
import { matchesApi } from "@/lib/api/matches"
import { matchKeys, tournamentKeys } from "@/lib/query-keys"
import { extractApiError } from "@/lib/api-error"
import type { UpdateMatchRequest } from "@/types/api/matches"

/**
 * Terminal match transition (complete or abandon). On success it refreshes the
 * match, its score/events, AND the parent tournament's standings + detail — so
 * a completed result propagates to the standings table immediately without any
 * SSE change or manual refresh.
 */
export function useCompleteMatch(orgSlug: string, matchId: string, tournamentId: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (body: UpdateMatchRequest) => matchesApi.update(orgSlug, matchId, body),
    onSuccess: (res) => {
      queryClient.setQueryData(matchKeys.detail(orgSlug, matchId), res.data)
      queryClient.invalidateQueries({ queryKey: matchKeys.score(orgSlug, matchId) })
      queryClient.invalidateQueries({ queryKey: matchKeys.eventsRoot(orgSlug, matchId) })
      if (tournamentId) {
        queryClient.invalidateQueries({ queryKey: tournamentKeys.standings(orgSlug, tournamentId) })
        queryClient.invalidateQueries({ queryKey: tournamentKeys.detail(orgSlug, tournamentId) })
        queryClient.invalidateQueries({ queryKey: tournamentKeys.lists(orgSlug) })
      }
    },
    onError: (err) => toast.error(extractApiError(err)),
  })
}
