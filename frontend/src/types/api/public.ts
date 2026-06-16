// Public (anonymous) tournament surface types — PRI-1. Mirrors internal/public DTOs.

export interface PublicOrganizationRef {
  name: string
  slug: string
}

export interface PublicTournament {
  name: string
  slug: string
  sport: string
  format: string
  participant_type: string
  status: string
  visibility: string
  banner_url?: string
  description?: string
  prize_pool?: string
  currency: string
  max_participants?: number
  registration_opens_at?: string
  registration_closes_at?: string
  starts_at?: string
  ends_at?: string
  venue?: string
  city?: string
  country?: string
  rules?: string
  organization: PublicOrganizationRef
}

export interface PublicSlot {
  kind: "participant" | "tbd" | "qualifier"
  name: string
}

export interface PublicMatch {
  id: string
  round_number?: number
  round_name?: string
  match_number?: number
  group_label?: string
  home: PublicSlot
  away: PublicSlot
  scheduled_at?: string
  venue?: string
  status: string
  home_score: number
  away_score: number
  is_walkover: boolean
  winner_name?: string
  next_match_id?: string
  next_match_slot?: number
}

export interface PublicMatchesResponse {
  matches: PublicMatch[]
}

export interface PublicStandingsRow {
  position: number
  participant_name: string
  played: number
  wins: number
  losses: number
  draws: number
  points: number
  score_for: number
  score_against: number
  score_difference: number
  disqualified: boolean
}

export interface PublicStandingsResponse {
  format: string
  status: string
  point_system: {
    win_points: number
    draw_points: number
    loss_points: number
    close_margin?: number
    close_loss_points?: number
  }
  standings: PublicStandingsRow[]
}

export interface PublicMatchEvent {
  sequence: number
  event_type: string
  period?: number
  clock_seconds?: number
  actor_name?: string
  recorded_at?: string
}

export interface PublicMatchDetail {
  id: string
  round_name?: string
  group_label?: string
  home: PublicSlot
  away: PublicSlot
  scheduled_at?: string
  started_at?: string
  ended_at?: string
  venue?: string
  status: string
  home_score: number
  away_score: number
  is_walkover: boolean
  winner_name?: string
  timeline: PublicMatchEvent[]
}
