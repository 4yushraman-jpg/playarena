"use client"

import api from "./client"
import type { Tournament, TournamentListParams } from "@/types/api/tournaments"

interface TournamentListResponse {
  tournaments: Tournament[]
  total: number
  limit: number
  offset: number
}

export const tournamentsApi = {
  list: (orgSlug: string, params?: TournamentListParams) =>
    api.get<TournamentListResponse>(
      `/api/v1/organizations/${orgSlug}/tournaments`,
      { params },
    ),
}
