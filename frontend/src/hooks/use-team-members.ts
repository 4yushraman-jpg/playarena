"use client"

import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"
import { teamsApi } from "@/lib/api/teams"
import { teamKeys } from "@/lib/query-keys"
import { extractApiError } from "@/lib/api-error"
import type { AddTeamMemberRequest } from "@/types/api/teams"

export function useTeamMembers(orgSlug: string, teamId: string) {
  return useQuery({
    queryKey: teamKeys.members(orgSlug, teamId),
    queryFn: () =>
      teamsApi.listMembers(orgSlug, teamId, { limit: 100 }).then((r) => r.data),
    staleTime: 30_000,
    enabled: !!orgSlug && !!teamId,
  })
}

export function useAddTeamMember(orgSlug: string, teamId: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (body: AddTeamMemberRequest) =>
      teamsApi.addMember(orgSlug, teamId, body),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: teamKeys.members(orgSlug, teamId) })
      toast.success("Member added")
    },
    onError: (err) => {
      toast.error(extractApiError(err))
    },
  })
}

export function useRemoveTeamMember(orgSlug: string, teamId: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (membershipId: string) =>
      teamsApi.removeMember(orgSlug, teamId, membershipId),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: teamKeys.members(orgSlug, teamId) })
      toast.success("Member removed")
    },
    onError: (err) => {
      toast.error(extractApiError(err))
    },
  })
}
