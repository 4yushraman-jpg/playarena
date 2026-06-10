// Backend PlayerRankingEntry: display_name (not player_name), no organization_id per entry
export interface PlayerRankingEntry {
  rank: number
  player_id: string
  display_name: string
  tournaments_played: number
  tournaments_won: number
  podium_finishes: number
  total_matches: number
  total_wins: number
  total_draws: number
  total_losses: number
  total_points: number
  win_rate: number
  score_for: number
  score_against: number
}

export interface TeamRankingEntry {
  rank: number
  team_id: string
  team_name: string
  tournaments_played: number
  tournaments_won: number
  podium_finishes: number
  total_matches: number
  total_wins: number
  total_draws: number
  total_losses: number
  total_points: number
  win_rate: number
  score_for: number
  score_against: number
}

export interface PlayerRankingsResponse {
  organization_id: string
  rankings: PlayerRankingEntry[]
  total: number
  limit: number
  offset: number
}

export interface TeamRankingsResponse {
  organization_id: string
  rankings: TeamRankingEntry[]
  total: number
  limit: number
  offset: number
}

export interface RankingListParams {
  limit?: number
  offset?: number
}
