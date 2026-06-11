"use client"

import api from "./client"
import type {
  TournamentRegistration,
  CreateRegistrationRequest,
  UpdateRegistrationRequest,
  RegistrationListParams,
} from "@/types/api/tournament-registrations"

interface RegistrationListResponse {
  registrations: TournamentRegistration[]
  total: number
  limit: number
  offset: number
}

export const registrationsApi = {
  list: (orgSlug: string, tournamentId: string, params?: RegistrationListParams) =>
    api.get<RegistrationListResponse>(
      `/api/v1/organizations/${orgSlug}/tournaments/${tournamentId}/registrations`,
      { params },
    ),

  getById: (orgSlug: string, tournamentId: string, registrationId: string) =>
    api.get<TournamentRegistration>(
      `/api/v1/organizations/${orgSlug}/tournaments/${tournamentId}/registrations/${registrationId}`,
    ),

  register: (orgSlug: string, tournamentId: string, body: CreateRegistrationRequest) =>
    api.post<TournamentRegistration>(
      `/api/v1/organizations/${orgSlug}/tournaments/${tournamentId}/registrations`,
      body,
    ),

  update: (orgSlug: string, tournamentId: string, registrationId: string, body: UpdateRegistrationRequest) =>
    api.patch<TournamentRegistration>(
      `/api/v1/organizations/${orgSlug}/tournaments/${tournamentId}/registrations/${registrationId}`,
      body,
    ),

  withdraw: (orgSlug: string, tournamentId: string, registrationId: string) =>
    api.delete(
      `/api/v1/organizations/${orgSlug}/tournaments/${tournamentId}/registrations/${registrationId}`,
    ),
}
