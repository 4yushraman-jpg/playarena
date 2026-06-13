import type { TournamentRegistration } from "@/types/api/tournament-registrations"
import type { CreateMatchRequest, UpdateMatchRequest } from "@/types/api/matches"
import type { FixtureParticipant, CoercedFixtureValues } from "./fixture-form"

// Pure mapping between registration/fixture-form data and the matches API.
// Isolated here so the team-vs-individual field routing — the part most likely
// to send the wrong slot to the backend — is unit-testable without the DOM.

/**
 * Projects approved registrations into selectable participants, picking the
 * team or player identity depending on the tournament format. Registrations
 * missing the relevant id are dropped.
 */
export function registrationsToParticipants(
  registrations: TournamentRegistration[],
  isTeam: boolean,
): FixtureParticipant[] {
  return registrations
    .map((r) =>
      isTeam
        ? { id: r.team_id ?? "", name: r.team_name ?? "Unknown team" }
        : { id: r.player_id ?? "", name: r.player_name ?? "Unknown player" },
    )
    .filter((p) => p.id !== "")
}

/**
 * Builds a create-match request, routing the home/away ids to the team slots
 * for team tournaments and the player slots for individual tournaments. The
 * backend rejects mixing the two (ErrMixedParticipantTypes).
 */
export function buildCreateMatchBody(
  tournamentId: string,
  isTeam: boolean,
  coerced: CoercedFixtureValues,
): CreateMatchRequest {
  return {
    tournament_id: tournamentId,
    scheduled_at: coerced.scheduledAt,
    venue: coerced.venue,
    round_name: coerced.roundName,
    round_number: coerced.roundNumber,
    match_number: coerced.matchNumber,
    ...(isTeam
      ? { home_team_id: coerced.homeId, away_team_id: coerced.awayId }
      : { home_player_id: coerced.homeId, away_player_id: coerced.awayId }),
  }
}

/**
 * Builds a patch-match request for a scheduled fixture. Cleared optional fields
 * are sent as null so the organizer can blank a venue/round; participant slots
 * use the format-appropriate keys.
 */
export function buildUpdateMatchBody(
  isTeam: boolean,
  coerced: CoercedFixtureValues,
): UpdateMatchRequest {
  return {
    scheduled_at: coerced.scheduledAt,
    venue: coerced.venue ?? null,
    round_name: coerced.roundName ?? null,
    round_number: coerced.roundNumber ?? null,
    match_number: coerced.matchNumber ?? null,
    ...(isTeam
      ? { home_team_id: coerced.homeId, away_team_id: coerced.awayId }
      : { home_player_id: coerced.homeId, away_player_id: coerced.awayId }),
  }
}
