"use client"

import api from "./client"
import type {
  Player,
  PlayerListParams,
  CreatePlayerRequest,
  UpdatePlayerRequest,
} from "@/types/api/players"

interface PlayerListResponse {
  players: Player[]
  total: number
  limit: number
  offset: number
}

export const playersApi = {
  list: (orgSlug: string, params?: PlayerListParams) =>
    api.get<PlayerListResponse>(
      `/api/v1/organizations/${orgSlug}/players`,
      { params },
    ),

  getById: (orgSlug: string, id: string) =>
    api.get<Player>(`/api/v1/organizations/${orgSlug}/players/${id}`),

  create: (orgSlug: string, body: CreatePlayerRequest) =>
    api.post<Player>(`/api/v1/organizations/${orgSlug}/players`, body),

  update: (orgSlug: string, id: string, body: UpdatePlayerRequest) =>
    api.patch<Player>(`/api/v1/organizations/${orgSlug}/players/${id}`, body),

  delete: (orgSlug: string, id: string) =>
    api.delete(`/api/v1/organizations/${orgSlug}/players/${id}`),
}
