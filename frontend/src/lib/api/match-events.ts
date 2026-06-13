"use client"

import api from "./client"
import type {
  MatchEvent,
  MatchEventListParams,
  CreateMatchEventRequest,
} from "@/types/api/match-events"

interface MatchEventListResponse {
  events: MatchEvent[]
  total: number
  limit: number
  offset: number
  effective_only: boolean
}

// Match-events API. The append-only event log is the source of truth for
// scoring. `create` appends a single event (scoring action or correction);
// sequence_number and recorded_by are assigned server-side.
export const matchEventsApi = {
  list: (orgSlug: string, matchId: string, params?: MatchEventListParams) =>
    api.get<MatchEventListResponse>(
      `/api/v1/organizations/${orgSlug}/matches/${matchId}/events`,
      { params },
    ),

  create: (orgSlug: string, matchId: string, body: CreateMatchEventRequest) =>
    api.post<MatchEvent>(
      `/api/v1/organizations/${orgSlug}/matches/${matchId}/events`,
      body,
    ),
}
