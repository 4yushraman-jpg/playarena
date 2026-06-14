// Fixture generation (FE-8C) request/response types.

export type SeedStrategy = "manual" | "random" | "registration"
export type GenerationFormat = "round_robin" | "league" | "knockout" | "group_knockout"

export interface GenerateRequest {
  format?: GenerationFormat
  seed_strategy: SeedStrategy
  // random_seed is a JS-safe integer (≤ 2^52) so it round-trips exactly. The
  // server returns the seed it used on a random preview; echo it on confirm to
  // reproduce the identical bracket.
  random_seed?: number
  legs?: number
  groups?: number
  qualifiers_per_group?: number
  expected_participants?: number
  dry_run?: boolean
}

export type SlotKind = "participant" | "tbd" | "qualifier"

export interface PreviewSlot {
  kind: SlotKind
  participant_id?: string
  group?: string
  rank?: number
}

export interface PreviewMatch {
  round_number: number
  round_name: string
  match_number: number
  group_label?: string
  home: PreviewSlot
  away: PreviewSlot
  next_match_number?: number
  next_match_slot?: number
}

export interface PreviewBye {
  participant_id: string
  into_round: number
}

export interface GenerateResponse {
  dry_run: boolean
  generated: number
  format: GenerationFormat
  seed_strategy: SeedStrategy
  random_seed?: number
  seed_order: string[]
  matches: PreviewMatch[]
  byes?: PreviewBye[]
  warnings?: string[]
  generated_at?: string
}

export interface GenerationInfo {
  generated: boolean
  generated_by?: string
  format?: GenerationFormat
  seed_strategy?: SeedStrategy
  random_seed?: number
  legs?: number
  groups?: number
  qualifiers_per_group?: number
  generated_at?: string
  match_count?: number
  seed_order?: string[]
}

export interface ResolveQualifiersResponse {
  slots_filled: number
}
