// Backend status transitions:
//   pending → approved | rejected
//   approved → withdrawn | disqualified
//   pending → withdrawn
//   Terminal: rejected, withdrawn, disqualified
export type RegistrationStatus =
  | "pending"
  | "approved"
  | "rejected"
  | "withdrawn"
  | "disqualified"

export interface TournamentRegistration {
  id: string
  tournament_id: string
  organization_id: string
  team_id: string | null
  player_id: string | null
  seed_number: number | null
  status: RegistrationStatus
  registered_by: string | null
  registered_at: string
  approved_by: string | null
  approved_at: string | null
  notes: string | null
  created_at: string
  updated_at: string
}

export interface CreateRegistrationRequest {
  team_id?: string
  player_id?: string
  notes?: string
}

export interface UpdateRegistrationRequest {
  status?: RegistrationStatus
  seed_number?: number | null
  notes?: string | null
}

export interface RegistrationListParams {
  limit?: number
  offset?: number
  status?: RegistrationStatus
}
