"use client"

import api from "./client"
import type {
  Match,
  MatchListParams,
  CreateMatchRequest,
  UpdateMatchRequest,
} from "@/types/api/matches"

interface MatchListResponse {
  matches: Match[]
  total: number
  limit: number
  offset: number
}

export const matchesApi = {
  list: (orgSlug: string, params?: MatchListParams) =>
    api.get<MatchListResponse>(`/api/v1/organizations/${orgSlug}/matches`, { params }),

  getById: (orgSlug: string, id: string) =>
    api.get<Match>(`/api/v1/organizations/${orgSlug}/matches/${id}`),

  create: (orgSlug: string, body: CreateMatchRequest) =>
    api.post<Match>(`/api/v1/organizations/${orgSlug}/matches`, body),

  update: (orgSlug: string, id: string, body: UpdateMatchRequest) =>
    api.patch<Match>(`/api/v1/organizations/${orgSlug}/matches/${id}`, body),

  // Soft-cancel: backend transitions status → cancelled, never hard-deletes.
  delete: (orgSlug: string, id: string) =>
    api.delete(`/api/v1/organizations/${orgSlug}/matches/${id}`),
}
