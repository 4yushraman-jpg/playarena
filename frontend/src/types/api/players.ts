// Backend players.Response status values
export type PlayerStatus = "active" | "inactive" | "injured" | "suspended" | "retired"

export interface Player {
  id: string
  organization_id: string
  user_id: string | null
  display_name: string
  jersey_number: string | null   // stored as string in backend
  position: string | null
  height_cm: number | null
  weight_kg: number | null
  dominant_hand: string | null   // "left" | "right" | "ambidextrous"
  date_of_birth: string | null   // YYYY-MM-DD
  nationality: string | null
  bio: string | null
  status: PlayerStatus
  created_at: string
  updated_at: string
}

export interface CreatePlayerRequest {
  display_name: string
  jersey_number?: string         // string, not number
  position?: string
  height_cm?: number
  weight_kg?: number
  dominant_hand?: string
  date_of_birth?: string
  nationality?: string
  bio?: string
  user_id?: string
}

export interface UpdatePlayerRequest {
  display_name?: string
  jersey_number?: string | null
  position?: string | null
  height_cm?: number | null
  weight_kg?: number | null
  dominant_hand?: string | null
  date_of_birth?: string | null
  nationality?: string | null
  bio?: string | null
  status?: PlayerStatus
}

export interface PlayerListParams {
  limit?: number
  offset?: number
  search?: string
  status?: PlayerStatus
}
