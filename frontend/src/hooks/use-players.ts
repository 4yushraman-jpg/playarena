"use client"

import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"
import { playersApi } from "@/lib/api/players"
import { playerKeys } from "@/lib/query-keys"
import { extractApiError } from "@/lib/api-error"
import type { PlayerListParams, CreatePlayerRequest, UpdatePlayerRequest } from "@/types/api/players"

export function usePlayerList(orgSlug: string, params?: PlayerListParams) {
  return useQuery({
    queryKey: playerKeys.list(orgSlug, params),
    queryFn: () => playersApi.list(orgSlug, params).then((r) => r.data),
    staleTime: 30_000,
    enabled: !!orgSlug,
  })
}

export function usePlayer(orgSlug: string, playerId: string) {
  return useQuery({
    queryKey: playerKeys.detail(orgSlug, playerId),
    queryFn: () => playersApi.getById(orgSlug, playerId).then((r) => r.data),
    staleTime: 30_000,
    enabled: !!orgSlug && !!playerId,
  })
}

export function useCreatePlayer(orgSlug: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (body: CreatePlayerRequest) => playersApi.create(orgSlug, body),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: playerKeys.all(orgSlug) })
      toast.success("Player created")
    },
    onError: (err) => {
      toast.error(extractApiError(err))
    },
  })
}

export function useUpdatePlayer(orgSlug: string, playerId: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (body: UpdatePlayerRequest) => playersApi.update(orgSlug, playerId, body),
    onSuccess: (response) => {
      queryClient.setQueryData(playerKeys.detail(orgSlug, playerId), response.data)
      queryClient.invalidateQueries({ queryKey: playerKeys.list(orgSlug) })
      toast.success("Player updated")
    },
    onError: (err) => {
      toast.error(extractApiError(err))
    },
  })
}

export function useDeletePlayer(orgSlug: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (playerId: string) => playersApi.delete(orgSlug, playerId),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: playerKeys.all(orgSlug) })
      toast.success("Player removed")
    },
    onError: (err) => {
      toast.error(extractApiError(err))
    },
  })
}
