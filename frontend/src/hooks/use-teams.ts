"use client"

import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"
import { teamsApi } from "@/lib/api/teams"
import { teamKeys } from "@/lib/query-keys"
import { extractApiError } from "@/lib/api-error"
import type { TeamListParams, CreateTeamRequest, UpdateTeamRequest } from "@/types/api/teams"

export function useTeamList(orgSlug: string, params?: TeamListParams) {
  return useQuery({
    queryKey: teamKeys.list(orgSlug, params),
    queryFn: () => teamsApi.list(orgSlug, params).then((r) => r.data),
    staleTime: 30_000,
    enabled: !!orgSlug,
  })
}

export function useTeam(orgSlug: string, teamId: string) {
  return useQuery({
    queryKey: teamKeys.detail(orgSlug, teamId),
    queryFn: () => teamsApi.getById(orgSlug, teamId).then((r) => r.data),
    staleTime: 30_000,
    enabled: !!orgSlug && !!teamId,
  })
}

export function useCreateTeam(orgSlug: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (body: CreateTeamRequest) => teamsApi.create(orgSlug, body),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: teamKeys.all(orgSlug) })
      toast.success("Team created")
    },
    onError: (err) => {
      toast.error(extractApiError(err))
    },
  })
}

export function useUpdateTeam(orgSlug: string, teamId: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (body: UpdateTeamRequest) => teamsApi.update(orgSlug, teamId, body),
    onSuccess: (response) => {
      queryClient.setQueryData(teamKeys.detail(orgSlug, teamId), response.data)
      queryClient.invalidateQueries({ queryKey: teamKeys.list(orgSlug) })
      toast.success("Team updated")
    },
    onError: (err) => {
      toast.error(extractApiError(err))
    },
  })
}

export function useDeleteTeam(orgSlug: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (teamId: string) => teamsApi.delete(orgSlug, teamId),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: teamKeys.all(orgSlug) })
      toast.success("Team removed")
    },
    onError: (err) => {
      toast.error(extractApiError(err))
    },
  })
}
