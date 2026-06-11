"use client"

import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"
import { registrationsApi } from "@/lib/api/registrations"
import { tournamentKeys } from "@/lib/query-keys"
import { extractApiError } from "@/lib/api-error"
import type {
  CreateRegistrationRequest,
  UpdateRegistrationRequest,
  RegistrationListParams,
  RegistrationStatus,
} from "@/types/api/tournament-registrations"

// Success copy keyed by the status a registration transitioned to.
const STATUS_SUCCESS_COPY: Partial<Record<RegistrationStatus, string>> = {
  approved: "Registration approved",
  rejected: "Registration rejected",
  withdrawn: "Registration withdrawn",
  disqualified: "Participant disqualified",
}

export function useRegistrationList(
  orgSlug: string,
  tournamentId: string,
  params?: RegistrationListParams,
  options?: { enabled?: boolean },
) {
  return useQuery({
    queryKey: tournamentKeys.registrationList(orgSlug, tournamentId, params),
    queryFn: () =>
      registrationsApi.list(orgSlug, tournamentId, params).then((r) => r.data),
    staleTime: 15_000,
    enabled: (options?.enabled ?? true) && !!orgSlug && !!tournamentId,
  })
}

export function useRegisterParticipant(orgSlug: string, tournamentId: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (body: CreateRegistrationRequest) =>
      registrationsApi.register(orgSlug, tournamentId, body),
    onSuccess: () => {
      // Detail carries registration_counts (capacity); invalidate alongside lists.
      queryClient.invalidateQueries({
        queryKey: tournamentKeys.registrations(orgSlug, tournamentId),
      })
      queryClient.invalidateQueries({
        queryKey: tournamentKeys.detail(orgSlug, tournamentId),
      })
      queryClient.invalidateQueries({ queryKey: tournamentKeys.lists(orgSlug) })
      toast.success("Registration submitted")
    },
    onError: (err) => {
      toast.error(extractApiError(err))
    },
  })
}

export function useUpdateRegistration(orgSlug: string, tournamentId: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({
      registrationId,
      body,
    }: {
      registrationId: string
      body: UpdateRegistrationRequest
    }) => registrationsApi.update(orgSlug, tournamentId, registrationId, body),
    onSuccess: (response, variables) => {
      const reg = response.data
      queryClient.setQueryData(
        tournamentKeys.registration(orgSlug, tournamentId, reg.id),
        reg,
      )
      // Lists for tab contents, detail for registration_counts/capacity,
      // tournament lists for the directory capacity column.
      queryClient.invalidateQueries({
        queryKey: tournamentKeys.registrations(orgSlug, tournamentId),
      })
      queryClient.invalidateQueries({
        queryKey: tournamentKeys.detail(orgSlug, tournamentId),
      })
      queryClient.invalidateQueries({ queryKey: tournamentKeys.lists(orgSlug) })

      const copy = variables.body.status
        ? STATUS_SUCCESS_COPY[variables.body.status]
        : undefined
      toast.success(copy ?? "Registration updated")
    },
    onError: (err) => {
      toast.error(extractApiError(err))
    },
  })
}

export function useWithdrawRegistration(orgSlug: string, tournamentId: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (registrationId: string) =>
      registrationsApi.withdraw(orgSlug, tournamentId, registrationId),
    onSuccess: () => {
      queryClient.invalidateQueries({
        queryKey: tournamentKeys.registrations(orgSlug, tournamentId),
      })
      queryClient.invalidateQueries({
        queryKey: tournamentKeys.detail(orgSlug, tournamentId),
      })
      queryClient.invalidateQueries({ queryKey: tournamentKeys.lists(orgSlug) })
      toast.success("Registration withdrawn")
    },
    onError: (err) => {
      toast.error(extractApiError(err))
    },
  })
}
