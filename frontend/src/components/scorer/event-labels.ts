import type { MatchEventType } from "@/types/api/match-events"

// Human-readable labels for the read-only timeline. Keys mirror the backend's
// 21 match_event_type ENUM values exactly.
export const EVENT_LABELS: Record<MatchEventType, string> = {
  match_started: "Match started",
  match_ended: "Match ended",
  half_started: "Half started",
  half_ended: "Half ended",
  timeout_called: "Timeout",
  timeout_ended: "Timeout ended",
  raid_attempt: "Raid attempt",
  raid_successful: "Successful raid",
  raid_empty: "Empty raid",
  bonus_point_awarded: "Bonus point",
  tackle_successful: "Successful tackle",
  super_tackle: "Super tackle",
  super_raid: "Super raid",
  do_or_die_raid: "Do-or-die raid",
  all_out: "All out",
  player_out: "Player out",
  player_revived: "Player revived",
  player_substituted: "Substitution",
  player_injured: "Player injured",
  penalty_awarded: "Penalty",
  score_correction: "Correction",
}

export type EventKind = "score" | "correction" | "lifecycle" | "state"

const SCORE_TYPES = new Set<MatchEventType>([
  "raid_successful",
  "bonus_point_awarded",
  "tackle_successful",
  "super_tackle",
  "all_out",
  "penalty_awarded",
])

const LIFECYCLE_TYPES = new Set<MatchEventType>([
  "match_started",
  "match_ended",
  "half_started",
  "half_ended",
  "timeout_called",
  "timeout_ended",
])

export function eventKind(type: MatchEventType): EventKind {
  if (type === "score_correction") return "correction"
  if (SCORE_TYPES.has(type)) return "score"
  if (LIFECYCLE_TYPES.has(type)) return "lifecycle"
  return "state"
}

export function eventLabel(type: MatchEventType): string {
  return EVENT_LABELS[type] ?? type
}
