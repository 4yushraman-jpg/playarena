"use client"

import api from "./client"
import type {
  GenerateRequest,
  GenerateResponse,
  GenerationInfo,
  ResolveQualifiersResponse,
} from "@/types/api/fixtures"

const base = (orgSlug: string, tournamentId: string) =>
  `/api/v1/organizations/${orgSlug}/tournaments/${tournamentId}/fixtures`

export const fixturesApi = {
  // generate handles both preview (dry_run) and confirmed generation.
  generate: (orgSlug: string, tournamentId: string, body: GenerateRequest) =>
    api.post<GenerateResponse>(`${base(orgSlug, tournamentId)}/generate`, body),

  resolveQualifiers: (orgSlug: string, tournamentId: string) =>
    api.post<ResolveQualifiersResponse>(`${base(orgSlug, tournamentId)}/resolve-qualifiers`, {}),

  generationInfo: (orgSlug: string, tournamentId: string) =>
    api.get<GenerationInfo>(`${base(orgSlug, tournamentId)}/generation`),
}
