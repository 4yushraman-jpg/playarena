"use client"

import api from "./client"
import type {
  Tournament,
  TournamentListParams,
  CreateTournamentRequest,
  UpdateTournamentRequest,
  StandingsResponse,
} from "@/types/api/tournaments"

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

  getById: (orgSlug: string, id: string) =>
    api.get<Tournament>(`/api/v1/organizations/${orgSlug}/tournaments/${id}`),

  create: (orgSlug: string, body: CreateTournamentRequest) =>
    api.post<Tournament>(`/api/v1/organizations/${orgSlug}/tournaments`, body),

  update: (orgSlug: string, id: string, body: UpdateTournamentRequest) =>
    api.patch<Tournament>(`/api/v1/organizations/${orgSlug}/tournaments/${id}`, body),

  delete: (orgSlug: string, id: string) =>
    api.delete(`/api/v1/organizations/${orgSlug}/tournaments/${id}`),

  getStandings: (orgSlug: string, id: string) =>
    api.get<StandingsResponse>(`/api/v1/organizations/${orgSlug}/tournaments/${id}/standings`),
}
