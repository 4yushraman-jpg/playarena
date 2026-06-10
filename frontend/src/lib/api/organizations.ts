"use client"

import api from "./client"
import type { Organization } from "@/types/api/organizations"

interface OrgListResponse {
  organizations: Organization[]
  total: number
  limit: number
  offset: number
}

export const orgsApi = {
  list: (params?: { limit?: number; offset?: number }) =>
    api.get<OrgListResponse>("/api/v1/organizations", { params }),

  getBySlug: (slug: string) =>
    api.get<Organization>(`/api/v1/organizations/${slug}`),

  create: (data: import("@/types/api/organizations").CreateOrganizationRequest) =>
    api.post<Organization>("/api/v1/organizations", data),

  update: (slug: string, data: import("@/types/api/organizations").UpdateOrganizationRequest) =>
    api.patch<Organization>(`/api/v1/organizations/${slug}`, data),

  delete: (slug: string) =>
    api.delete(`/api/v1/organizations/${slug}`),
}
