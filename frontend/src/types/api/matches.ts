export type MatchStatus =
  | "scheduled"
  | "live"
  | "completed"
  | "cancelled"
  | "abandoned"
  | "walkover"

export interface Match {
  id: string
  organization_id: string
  tournament_id: string
  round_number: number | null
  round_name: string | null
  match_number: number | null
  // Team-format participants
  home_team_id: string | null
  away_team_id: string | null
  // Individual-format participants
  home_player_id: string | null
  away_player_id: string | null
  venue: string | null
  scheduled_at: string | null
  started_at: string | null
  ended_at: string | null
  status: MatchStatus
  winner_team_id: string | null
  winner_player_id: string | null
  is_walkover: boolean
  home_score: number
  away_score: number
  notes: string | null
  created_at: string
  updated_at: string
}

export interface LiveScore {
  match_id: string
  match_status: MatchStatus
  home_score: number
  away_score: number
  home_team_id: string | null
  away_team_id: string | null
  home_player_id: string | null
  away_player_id: string | null
  is_walkover: boolean
}

export interface CreateMatchRequest {
  tournament_id: string
  round_number?: number
  round_name?: string
  match_number?: number
  home_team_id?: string
  away_team_id?: string
  home_player_id?: string
  away_player_id?: string
  venue?: string
  scheduled_at: string         // required by backend
  notes?: string
}

export interface UpdateMatchRequest {
  round_number?: number | null
  round_name?: string | null
  match_number?: number | null
  home_team_id?: string | null
  away_team_id?: string | null
  home_player_id?: string | null
  away_player_id?: string | null
  venue?: string | null
  scheduled_at?: string | null
  status?: MatchStatus
  winner_team_id?: string | null
  winner_player_id?: string | null
  notes?: string | null
}

// Award an administrative win when one side does not appear. The winner string
// names the present side; the backend resolves it to the concrete participant.
export interface WalkoverRequest {
  winner: "home" | "away"
  reason: string
}

export interface MatchListParams {
  limit?: number
  offset?: number
  tournament_id?: string
  status?: MatchStatus
  search?: string
}
