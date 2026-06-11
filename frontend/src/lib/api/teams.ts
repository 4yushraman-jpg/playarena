"use client"

import api from "./client"
import type {
  Team,
  TeamMember,
  TeamListParams,
  CreateTeamRequest,
  UpdateTeamRequest,
  AddTeamMemberRequest,
} from "@/types/api/teams"

interface TeamListResponse {
  teams: Team[]
  total: number
  limit: number
  offset: number
}

interface TeamMemberListResponse {
  members: TeamMember[]
}

export const teamsApi = {
  list: (orgSlug: string, params?: TeamListParams) =>
    api.get<TeamListResponse>(
      `/api/v1/organizations/${orgSlug}/teams`,
      { params },
    ),

  getById: (orgSlug: string, id: string) =>
    api.get<Team>(`/api/v1/organizations/${orgSlug}/teams/${id}`),

  create: (orgSlug: string, body: CreateTeamRequest) =>
    api.post<Team>(`/api/v1/organizations/${orgSlug}/teams`, body),

  update: (orgSlug: string, id: string, body: UpdateTeamRequest) =>
    api.patch<Team>(`/api/v1/organizations/${orgSlug}/teams/${id}`, body),

  delete: (orgSlug: string, id: string) =>
    api.delete(`/api/v1/organizations/${orgSlug}/teams/${id}`),

  listMembers: (orgSlug: string, teamId: string, params?: { limit?: number; offset?: number }) =>
    api.get<TeamMemberListResponse>(
      `/api/v1/organizations/${orgSlug}/teams/${teamId}/members`,
      { params },
    ),

  addMember: (orgSlug: string, teamId: string, body: AddTeamMemberRequest) =>
    api.post<TeamMember>(
      `/api/v1/organizations/${orgSlug}/teams/${teamId}/members`,
      body,
    ),

  removeMember: (orgSlug: string, teamId: string, membershipId: string) =>
    api.delete(
      `/api/v1/organizations/${orgSlug}/teams/${teamId}/members/${membershipId}`,
    ),
}
