/**
 * Centralized query key factory.
 *
 * Rules:
 * 1. Every key is org-scoped to prevent cross-org cache leakage.
 * 2. Keys are const-asserted tuples so TypeScript can narrow them.
 * 3. On org switch, call queryClient.clear() to flush all org-scoped data.
 *
 * Usage:
 *   useQuery({ queryKey: tournamentKeys.list(slug, { status: 'ongoing' }), ... })
 *   queryClient.invalidateQueries({ queryKey: tournamentKeys.all(slug) })
 */

import type { TournamentListParams } from "@/types/api/tournaments"
import type { MatchListParams } from "@/types/api/matches"
import type { PlayerListParams } from "@/types/api/players"
import type { TeamListParams } from "@/types/api/teams"
import type { RegistrationListParams } from "@/types/api/tournament-registrations"
import type { MatchEventListParams } from "@/types/api/match-events"
import type { NotificationListParams } from "@/types/api/notifications"
import type { RankingListParams } from "@/types/api/rankings"
import type { MemberListParams } from "@/types/api/members"
import type { UserListParams } from "@/types/api/users"
import type { MediaListParams } from "@/types/api/media"

// ── Me / global PlayerProfile (GP-1) ──────────────────────────────────────────
// NOT org-scoped. These keys must survive an org switch (do not clear on switch),
// because the global player identity is independent of any organization.

export const meKeys = {
  all: () => ["me"] as const,
  player: () => [...meKeys.all(), "player"] as const,
}

// Global (non-org) player profile read by id.
export const playerProfileKeys = {
  all: () => ["player-profile"] as const,
  detail: (id: string) => [...playerProfileKeys.all(), id] as const,
}

// ── Organizations ─────────────────────────────────────────────────────────────

export const orgKeys = {
  all: () => ["organizations"] as const,
  list: (params?: { search?: string; limit?: number; offset?: number }) =>
    [...orgKeys.all(), "list", params] as const,
  detail: (slug: string) => [...orgKeys.all(), slug] as const,
}

// ── Players ───────────────────────────────────────────────────────────────────

export const playerKeys = {
  all: (orgSlug: string) => ["players", orgSlug] as const,
  list: (orgSlug: string, params?: PlayerListParams) =>
    [...playerKeys.all(orgSlug), "list", params] as const,
  detail: (orgSlug: string, id: string) =>
    [...playerKeys.all(orgSlug), id] as const,
}

// ── Teams ─────────────────────────────────────────────────────────────────────

export const teamKeys = {
  all: (orgSlug: string) => ["teams", orgSlug] as const,
  list: (orgSlug: string, params?: TeamListParams) =>
    [...teamKeys.all(orgSlug), "list", params] as const,
  detail: (orgSlug: string, id: string) =>
    [...teamKeys.all(orgSlug), id] as const,
  members: (orgSlug: string, teamId: string) =>
    [...teamKeys.detail(orgSlug, teamId), "members"] as const,
}

// ── Tournaments ───────────────────────────────────────────────────────────────

// NOTE on invalidation: TanStack's partial key matching compares every element
// of the filter key, so a filter ending in `undefined` does NOT match a stored
// key ending in a params object. Always invalidate with a params-less root key
// (`lists`, `registrations`, `detail`) — never with `list(org)` / a key whose
// trailing params slot is undefined.
export const tournamentKeys = {
  all: (orgSlug: string) => ["tournaments", orgSlug] as const,
  /** Root for every tournament list query — use for invalidation. */
  lists: (orgSlug: string) => [...tournamentKeys.all(orgSlug), "list"] as const,
  list: (orgSlug: string, params?: TournamentListParams) =>
    [...tournamentKeys.lists(orgSlug), params ?? {}] as const,
  detail: (orgSlug: string, id: string) =>
    [...tournamentKeys.all(orgSlug), id] as const,
  standings: (orgSlug: string, id: string) =>
    [...tournamentKeys.detail(orgSlug, id), "standings"] as const,
  /** Root for everything registration-scoped under a tournament — use for invalidation. */
  registrations: (orgSlug: string, tournamentId: string) =>
    [...tournamentKeys.detail(orgSlug, tournamentId), "registrations"] as const,
  registrationList: (orgSlug: string, tournamentId: string, params?: RegistrationListParams) =>
    [...tournamentKeys.registrations(orgSlug, tournamentId), "list", params ?? {}] as const,
  registration: (orgSlug: string, tournamentId: string, registrationId: string) =>
    [...tournamentKeys.registrations(orgSlug, tournamentId), "detail", registrationId] as const,
}

// ── Matches ───────────────────────────────────────────────────────────────────

export const matchKeys = {
  all: (orgSlug: string) => ["matches", orgSlug] as const,
  list: (orgSlug: string, params?: MatchListParams) =>
    [...matchKeys.all(orgSlug), "list", params] as const,
  detail: (orgSlug: string, id: string) =>
    [...matchKeys.all(orgSlug), id] as const,
  score: (orgSlug: string, id: string) =>
    [...matchKeys.detail(orgSlug, id), "score"] as const,
  events: (orgSlug: string, matchId: string, params?: MatchEventListParams) =>
    [...matchKeys.detail(orgSlug, matchId), "events", params] as const,
  event: (orgSlug: string, matchId: string, eventId: string) =>
    [...matchKeys.detail(orgSlug, matchId), "events", eventId] as const,
}

// ── Notifications ─────────────────────────────────────────────────────────────

export const notificationKeys = {
  all: (orgSlug: string) => ["notifications", orgSlug] as const,
  list: (orgSlug: string, params?: NotificationListParams) =>
    [...notificationKeys.all(orgSlug), "list", params] as const,
  detail: (orgSlug: string, id: string) =>
    [...notificationKeys.all(orgSlug), id] as const,
  preferences: (orgSlug: string) =>
    [...notificationKeys.all(orgSlug), "preferences"] as const,
}

// ── Rankings ──────────────────────────────────────────────────────────────────

export const rankingKeys = {
  all: (orgSlug: string) => ["rankings", orgSlug] as const,
  players: (orgSlug: string, params?: RankingListParams) =>
    [...rankingKeys.all(orgSlug), "players", params] as const,
  teams: (orgSlug: string, params?: RankingListParams) =>
    [...rankingKeys.all(orgSlug), "teams", params] as const,
}

// ── Members (role assignment) ─────────────────────────────────────────────────

export const memberKeys = {
  all: (orgSlug: string) => ["members", orgSlug] as const,
  list: (orgSlug: string, params?: MemberListParams) =>
    [...memberKeys.all(orgSlug), "list", params] as const,
  userGrants: (orgSlug: string, userId: string) =>
    [...memberKeys.all(orgSlug), userId, "grants"] as const,
}

// ── Webhooks ──────────────────────────────────────────────────────────────────

export const webhookKeys = {
  all: (orgSlug: string) => ["webhooks", orgSlug] as const,
  list: (orgSlug: string) => [...webhookKeys.all(orgSlug), "list"] as const,
  detail: (orgSlug: string, id: string) =>
    [...webhookKeys.all(orgSlug), id] as const,
}

// ── Media ─────────────────────────────────────────────────────────────────────

export const mediaKeys = {
  all: (orgSlug: string) => ["media", orgSlug] as const,
  list: (orgSlug: string, params?: MediaListParams) =>
    [...mediaKeys.all(orgSlug), "list", params] as const,
  detail: (orgSlug: string, id: string) =>
    [...mediaKeys.all(orgSlug), id] as const,
}

// ── Users ─────────────────────────────────────────────────────────────────────

export const userKeys = {
  all: () => ["users"] as const,
  list: (params?: UserListParams) => [...userKeys.all(), "list", params] as const,
  detail: (id: string) => [...userKeys.all(), id] as const,
  me: () => [...userKeys.all(), "me"] as const,
}
