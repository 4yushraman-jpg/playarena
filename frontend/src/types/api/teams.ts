// Backend teams.Response status: active | inactive | disbanded
export type TeamStatus = "active" | "inactive" | "disbanded"
// Backend teams.MembershipResponse status: active | released
export type MembershipStatus = "active" | "released"

export interface Team {
  id: string
  organization_id: string
  name: string
  slug: string
  short_name: string | null
  description: string | null
  logo_url: string | null
  home_city: string | null
  home_venue: string | null
  founded_year: number | null
  primary_color: string | null
  secondary_color: string | null
  status: TeamStatus
  created_at: string
  updated_at: string
}

export interface TeamMember {
  id: string               // membership id
  team_id: string
  player_id: string
  organization_id: string
  player_display_name: string  // embedded from players JOIN; use this for all UI display
  role: string             // e.g. "player", "captain"
  jersey_number: string | null
  status: MembershipStatus
  joined_at: string
  left_at: string | null
  notes: string | null
  created_at: string
  updated_at: string
}

export interface CreateTeamRequest {
  name: string
  short_name?: string
  description?: string
  logo_url?: string
  home_city?: string
  home_venue?: string
  founded_year?: number
  primary_color?: string
  secondary_color?: string
}

export interface UpdateTeamRequest {
  name?: string
  short_name?: string | null
  description?: string | null
  logo_url?: string | null
  home_city?: string | null
  home_venue?: string | null
  founded_year?: number | null
  primary_color?: string | null
  secondary_color?: string | null
  status?: TeamStatus
}

export interface AddTeamMemberRequest {
  player_id: string
  role?: string
  jersey_number?: string
  notes?: string
}

export interface TeamListParams {
  limit?: number
  offset?: number
  search?: string
  status?: TeamStatus
}
