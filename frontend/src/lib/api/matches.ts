"use client"

import api from "./client"
import type { Match, MatchListParams } from "@/types/api/matches"

interface MatchListResponse {
  matches: Match[]
  total: number
  limit: number
  offset: number
}

export const matchesApi = {
  list: (orgSlug: string, params?: MatchListParams) =>
    api.get<MatchListResponse>(`/api/v1/organizations/${orgSlug}/matches`, { params }),
}
