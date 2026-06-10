export type OrganizationType = "club" | "federation" | "school" | "corporate" | "independent"

export interface Organization {
  id: string
  name: string
  slug: string
  type: OrganizationType
  status: string
  description: string | null
  website: string | null
  email: string | null
  phone: string | null
  country: string | null
  city: string | null
  created_at: string
  updated_at: string
}

export interface CreateOrganizationRequest {
  name: string
  type: OrganizationType
  description?: string
  website?: string
  email?: string
  phone?: string
  country?: string
  city?: string
}

export interface UpdateOrganizationRequest {
  name?: string
  type?: OrganizationType
  description?: string | null
  website?: string | null
  email?: string | null
  phone?: string | null
  country?: string | null
  city?: string | null
}
