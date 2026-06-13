"use client"

import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"
import { matchesApi } from "@/lib/api/matches"
import { matchKeys, tournamentKeys } from "@/lib/query-keys"
import { extractApiError } from "@/lib/api-error"
import type {
  MatchListParams,
  CreateMatchRequest,
  UpdateMatchRequest,
  WalkoverRequest,
} from "@/types/api/matches"

export function useMatchList(orgSlug: string, params?: MatchListParams) {
  return useQuery({
    queryKey: matchKeys.list(orgSlug, params),
    queryFn: () => matchesApi.list(orgSlug, params).then((r) => r.data),
    staleTime: 30_000,
    enabled: !!orgSlug,
  })
}

export function useMatch(orgSlug: string, matchId: string) {
  return useQuery({
    queryKey: matchKeys.detail(orgSlug, matchId),
    queryFn: () => matchesApi.getById(orgSlug, matchId).then((r) => r.data),
    staleTime: 30_000,
    enabled: !!orgSlug && !!matchId,
  })
}

export function useCreateMatch(orgSlug: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (body: CreateMatchRequest) => matchesApi.create(orgSlug, body),
    onSuccess: () => {
      // matchKeys.all covers every list and detail under the org.
      queryClient.invalidateQueries({ queryKey: matchKeys.all(orgSlug) })
      toast.success("Fixture created")
    },
    onError: (err) => {
      toast.error(extractApiError(err))
    },
  })
}

export function useUpdateMatch(orgSlug: string, matchId: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (body: UpdateMatchRequest) => matchesApi.update(orgSlug, matchId, body),
    onSuccess: (response) => {
      // PATCH returns the full match shape — seed the detail cache directly,
      // then invalidate lists so the directory reflects the edit.
      queryClient.setQueryData(matchKeys.detail(orgSlug, matchId), response.data)
      queryClient.invalidateQueries({ queryKey: matchKeys.lists(orgSlug) })
      toast.success("Fixture updated")
    },
    onError: (err) => {
      toast.error(extractApiError(err))
    },
  })
}

export function useWalkoverMatch(orgSlug: string, matchId: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (body: WalkoverRequest) => matchesApi.walkover(orgSlug, matchId, body),
    onSuccess: (response) => {
      const match = response.data
      // The endpoint returns the full match shape — seed the detail cache,
      // then invalidate lists so the fixture directory reflects the walkover.
      queryClient.setQueryData(matchKeys.detail(orgSlug, matchId), match)
      queryClient.invalidateQueries({ queryKey: matchKeys.lists(orgSlug) })
      // A walkover changes the standings table — invalidate the parent
      // tournament's standings (and detail, whose counts/health may shift).
      if (match.tournament_id) {
        queryClient.invalidateQueries({
          queryKey: tournamentKeys.standings(orgSlug, match.tournament_id),
        })
        queryClient.invalidateQueries({
          queryKey: tournamentKeys.detail(orgSlug, match.tournament_id),
        })
      }
      toast.success("Walkover awarded")
    },
    onError: (err) => {
      toast.error(extractApiError(err))
    },
  })
}

export function useDeleteMatch(orgSlug: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (matchId: string) => matchesApi.delete(orgSlug, matchId),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: matchKeys.all(orgSlug) })
      toast.success("Fixture cancelled")
    },
    onError: (err) => {
      toast.error(extractApiError(err))
    },
  })
}
