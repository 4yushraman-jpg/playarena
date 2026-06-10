export type MatchEventType =
  // Lifecycle
  | "match_started"
  | "match_ended"
  | "half_started"
  | "half_ended"
  | "timeout_called"
  | "timeout_ended"
  // Raid
  | "raid_attempt"
  | "raid_successful"
  | "raid_empty"
  | "bonus_point_awarded"
  // Tackle
  | "tackle_successful"
  | "super_tackle"
  // Compound
  | "super_raid"
  | "do_or_die_raid"
  | "all_out"
  // Player state
  | "player_out"
  | "player_revived"
  | "player_substituted"
  | "player_injured"
  // Administrative
  | "penalty_awarded"
  | "score_correction"

export interface MatchEvent {
  id: string
  match_id: string
  organization_id: string
  sequence_number: number
  event_type: MatchEventType
  team_id: string | null
  player_id: string | null
  period: number | null
  clock_seconds: number | null
  payload: Record<string, unknown>
  recorded_by: string | null
  recorded_at: string
  cancels_event_id: string | null
  created_at: string
}

export interface CreateMatchEventRequest {
  event_type: MatchEventType
  team_id?: string
  player_id?: string
  period?: number
  clock_seconds?: number
  payload?: Record<string, unknown>
  recorded_at?: string
  cancels_event_id?: string
}

export interface MatchEventListParams {
  limit?: number
  offset?: number
  effective_only?: boolean
}

// Payload shapes for events that carry structured data
export interface RaidSuccessfulPayload { points: number }
export interface PenaltyAwardedPayload { points: number }
export interface AllOutPayload { team_id: string; bonus_points: number }
