"use client"

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"
import { fixturesApi } from "@/lib/api/fixtures"
import { matchKeys, tournamentKeys } from "@/lib/query-keys"
import { extractApiError } from "@/lib/api-error"
import type { GenerateRequest } from "@/types/api/fixtures"

// usePreviewFixtures runs a dry-run generation: it returns the full structure
// WITHOUT persisting, so the wizard can show exactly what will be created.
export function usePreviewFixtures(orgSlug: string, tournamentId: string) {
  return useMutation({
    mutationFn: (body: Omit<GenerateRequest, "dry_run">) =>
      fixturesApi.generate(orgSlug, tournamentId, { ...body, dry_run: true }).then((r) => r.data),
    onError: (err) => toast.error(extractApiError(err)),
  })
}

// useGenerateFixtures persists the fixtures (confirm step).
export function useGenerateFixtures(orgSlug: string, tournamentId: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (body: Omit<GenerateRequest, "dry_run">) =>
      fixturesApi.generate(orgSlug, tournamentId, { ...body, dry_run: false }).then((r) => r.data),
    onSuccess: (res) => {
      queryClient.invalidateQueries({ queryKey: matchKeys.all(orgSlug) })
      queryClient.invalidateQueries({ queryKey: tournamentKeys.detail(orgSlug, tournamentId) })
      queryClient.invalidateQueries({ queryKey: tournamentKeys.generation(orgSlug, tournamentId) })
      toast.success(`Generated ${res.generated} fixtures`)
    },
    onError: (err) => toast.error(extractApiError(err)),
  })
}

// useResolveQualifiers fills the knockout slots of a group_knockout once the
// group stage is complete.
export function useResolveQualifiers(orgSlug: string, tournamentId: string) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: () => fixturesApi.resolveQualifiers(orgSlug, tournamentId).then((r) => r.data),
    onSuccess: (res) => {
      queryClient.invalidateQueries({ queryKey: matchKeys.all(orgSlug) })
      queryClient.invalidateQueries({ queryKey: tournamentKeys.standings(orgSlug, tournamentId) })
      toast.success(`Filled ${res.slots_filled} knockout slot${res.slots_filled === 1 ? "" : "s"}`)
    },
    onError: (err) => toast.error(extractApiError(err)),
  })
}

// useGenerationInfo reads the stored explainability record (audit trail).
export function useGenerationInfo(orgSlug: string, tournamentId: string, enabled = true) {
  return useQuery({
    queryKey: tournamentKeys.generation(orgSlug, tournamentId),
    queryFn: () => fixturesApi.generationInfo(orgSlug, tournamentId).then((r) => r.data),
    enabled: enabled && !!orgSlug && !!tournamentId,
    staleTime: 60_000,
  })
}
