export type TournamentStatus =
  | "draft"
  | "registration_open"
  | "registration_closed"
  | "ongoing"
  | "completed"
  | "cancelled"

export type TournamentFormat =
  | "league"
  | "knockout"
  | "group_knockout"
  | "round_robin"
  | "double_elimination"

export type ParticipantType = "team" | "individual"

// Per-status registration breakdown embedded in tournament responses.
// `active` = pending + approved — the count the backend enforces against
// max_participants. Always use `active` for capacity math, never `approved`.
export interface RegistrationCounts {
  pending: number
  approved: number
  rejected: number
  withdrawn: number
  disqualified: number
  active: number
  total: number
}

export interface Tournament {
  id: string
  organization_id: string
  name: string
  slug: string
  sport: string
  format: TournamentFormat
  status: TournamentStatus
  participant_type: ParticipantType
  description: string | null
  banner_url: string | null
  prize_pool: string | null    // decimal string e.g. "10000.00"
  currency: string             // ISO 4217, default "INR"
  max_participants: number | null
  min_participants: number | null
  registration_opens_at: string | null
  registration_closes_at: string | null
  starts_at: string | null
  ends_at: string | null
  venue: string | null
  city: string | null
  country: string | null       // ISO 3166-1 alpha-2
  rules: string | null
  created_by: string | null
  created_at: string
  updated_at: string
  // Present on create/get/list/update responses (omitted by older endpoints).
  registration_counts?: RegistrationCounts
}

// ── Standings ─────────────────────────────────────────────────────────────────

export interface StandingsRow {
  position: number
  participant_id: string
  // Team name or player display name resolved by the backend.
  // Empty string when the participant record can no longer be found.
  participant_name: string
  played: number
  wins: number
  losses: number
  draws: number
  points: number
  score_for: number
  score_against: number
  score_difference: number
}

// Backend tournaments.StandingsResponse shape
export interface StandingsResponse {
  tournament_id: string
  tournament_name: string
  format: string
  status: string
  point_system: {
    win_points: number
    draw_points: number
    loss_points: number
    close_margin?: number
    close_loss_points?: number
  }
  standings: StandingsRow[]
}

// ── Requests ──────────────────────────────────────────────────────────────────

export interface CreateTournamentRequest {
  name: string
  sport: string
  format: TournamentFormat
  participant_type?: ParticipantType
  description?: string
  banner_url?: string
  prize_pool?: string
  currency?: string
  max_participants?: number
  min_participants?: number
  registration_opens_at?: string
  registration_closes_at?: string
  starts_at?: string
  ends_at?: string
  venue?: string
  city?: string
  country?: string
  rules?: string
}

export interface UpdateTournamentRequest {
  name?: string
  sport?: string
  format?: TournamentFormat
  participant_type?: ParticipantType
  description?: string | null
  banner_url?: string | null
  prize_pool?: string | null
  currency?: string
  max_participants?: number | null
  min_participants?: number | null
  registration_opens_at?: string | null
  registration_closes_at?: string | null
  starts_at?: string | null
  ends_at?: string | null
  venue?: string | null
  city?: string | null
  country?: string | null
  rules?: string | null
  status?: TournamentStatus
}

export interface TournamentListParams {
  limit?: number
  offset?: number
  search?: string
  status?: TournamentStatus
}
