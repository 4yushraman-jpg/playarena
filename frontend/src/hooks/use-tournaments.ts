"use client"

import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"
import { tournamentsApi } from "@/lib/api/tournaments"
import { tournamentKeys } from "@/lib/query-keys"
import { extractApiError } from "@/lib/api-error"
import type {
  TournamentListParams,
  CreateTournamentRequest,
  UpdateTournamentRequest,
} from "@/types/api/tournaments"

export function useTournamentList(orgSlug: string, params?: TournamentListParams) {
  return useQuery({
    queryKey: tournamentKeys.list(orgSlug, params),
    queryFn: () => tournamentsApi.list(orgSlug, params).then((r) => r.data),
    staleTime: 30_000,
    enabled: !!orgSlug,
  })
}

export function useTournament(orgSlug: string, tournamentId: string) {
  return useQuery({
    queryKey: tournamentKeys.detail(orgSlug, tournamentId),
    queryFn: () => tournamentsApi.getById(orgSlug, tournamentId).then((r) => r.data),
    staleTime: 30_000,
    enabled: !!orgSlug && !!tournamentId,
  })
}

export function useTournamentStandings(orgSlug: string, tournamentId: string) {
  return useQuery({
    queryKey: tournamentKeys.standings(orgSlug, tournamentId),
    queryFn: () => tournamentsApi.getStandings(orgSlug, tournamentId).then((r) => r.data),
    staleTime: 60_000,
    enabled: !!orgSlug && !!tournamentId,
  })
}

export function useCreateTournament(orgSlug: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (body: CreateTournamentRequest) => tournamentsApi.create(orgSlug, body),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: tournamentKeys.all(orgSlug) })
      toast.success("Tournament created")
    },
    onError: (err) => {
      toast.error(extractApiError(err))
    },
  })
}

export function useUpdateTournament(orgSlug: string, tournamentId: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (body: UpdateTournamentRequest) =>
      tournamentsApi.update(orgSlug, tournamentId, body),
    onSuccess: (response) => {
      // PATCH responses carry the full tournament shape including
      // registration_counts, so the detail cache stays shape-consistent.
      queryClient.setQueryData(tournamentKeys.detail(orgSlug, tournamentId), response.data)
      queryClient.invalidateQueries({ queryKey: tournamentKeys.lists(orgSlug) })
    },
    onError: (err) => {
      toast.error(extractApiError(err))
    },
  })
}

export function useDeleteTournament(orgSlug: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (tournamentId: string) => tournamentsApi.delete(orgSlug, tournamentId),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: tournamentKeys.all(orgSlug) })
      toast.success("Tournament cancelled")
    },
    onError: (err) => {
      toast.error(extractApiError(err))
    },
  })
}
