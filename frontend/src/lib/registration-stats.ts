import type { Tournament, RegistrationCounts } from "@/types/api/tournaments"
import type { TournamentRegistration } from "@/types/api/tournament-registrations"

export const EMPTY_REGISTRATION_COUNTS: RegistrationCounts = {
  pending: 0,
  approved: 0,
  rejected: 0,
  withdrawn: 0,
  disqualified: 0,
  active: 0,
  total: 0,
}

/**
 * Server-computed registration counts with a zeroed fallback for responses
 * from before the counts were introduced.
 */
export function getRegistrationCounts(
  tournament: Pick<Tournament, "registration_counts">,
): RegistrationCounts {
  return tournament.registration_counts ?? EMPTY_REGISTRATION_COUNTS
}

export interface CapacityUsage {
  /** pending + approved — the number the backend enforces capacity against. */
  used: number
  max: number
  pct: number
  isFull: boolean
}

/**
 * Capacity usage for a tournament, or null when it has no participant cap.
 *
 * `used` is the ACTIVE count (pending + approved), matching the backend's
 * CountActiveRegistrations capacity check — an approved-only count would
 * under-report how close the tournament is to rejecting new registrations.
 */
export function getCapacityUsage(
  tournament: Pick<Tournament, "max_participants" | "registration_counts">,
): CapacityUsage | null {
  const max = tournament.max_participants
  if (!max || max <= 0) return null
  const used = getRegistrationCounts(tournament).active
  return {
    used,
    max,
    pct: Math.min(100, Math.round((used / max) * 100)),
    isFull: used >= max,
  }
}

/**
 * Directory-column capacity label: "3 / 16", or "5 registered" when the
 * tournament has no cap. Never renders a placeholder dash for the used count.
 */
export function formatCapacityLabel(
  tournament: Pick<Tournament, "max_participants" | "registration_counts">,
): string {
  const counts = getRegistrationCounts(tournament)
  const usage = getCapacityUsage(tournament)
  if (!usage) {
    return counts.active === 1 ? "1 registered" : `${counts.active} registered`
  }
  return `${usage.used} / ${usage.max}`
}

/**
 * Human label for a registration row. Prefers the server-joined display name;
 * falls back to a shortened identifier only if the join ever returns nothing.
 */
export function getParticipantLabel(
  registration: Pick<
    TournamentRegistration,
    "team_id" | "player_id" | "team_name" | "player_name"
  >,
): string {
  if (registration.team_name) return registration.team_name
  if (registration.player_name) return registration.player_name
  const id = registration.team_id ?? registration.player_id
  if (!id) return "Unknown participant"
  const kind = registration.team_id ? "Team" : "Player"
  return `${kind} ${id.slice(0, 8)}`
}
