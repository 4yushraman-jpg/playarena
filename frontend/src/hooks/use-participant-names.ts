"use client"

import { useMemo } from "react"
import { useTeamList } from "./use-teams"
import { usePlayerList } from "./use-players"

// The matches API returns participant UUIDs only (no embedded names). To render
// readable fixtures we resolve those UUIDs against the org's teams and players.
// A single bounded fetch of each list (cached by React Query and shared across
// the app) keeps this O(1) per page rather than per-row — no N+1 lookups.
//
// RESOLVE_LIMIT bounds the map; orgs larger than this still resolve every name
// that appears in the tournament-scoped fixtures view (those participants are
// always among the org's teams/players), and unresolved ids degrade to a short
// id rather than throwing.
const RESOLVE_LIMIT = 200

export interface ParticipantNameResolver {
  resolve: (teamId: string | null, playerId: string | null) => string
  isLoading: boolean
}

export function useParticipantNames(orgSlug: string): ParticipantNameResolver {
  const teamsQuery = useTeamList(orgSlug, { limit: RESOLVE_LIMIT })
  const playersQuery = usePlayerList(orgSlug, { limit: RESOLVE_LIMIT })

  const { teamMap, playerMap } = useMemo(() => {
    const teamMap = new Map<string, string>()
    for (const t of teamsQuery.data?.teams ?? []) teamMap.set(t.id, t.name)
    const playerMap = new Map<string, string>()
    for (const p of playersQuery.data?.players ?? []) playerMap.set(p.id, p.display_name)
    return { teamMap, playerMap }
  }, [teamsQuery.data, playersQuery.data])

  const isLoading = teamsQuery.isLoading || playersQuery.isLoading

  function resolve(teamId: string | null, playerId: string | null): string {
    const id = teamId ?? playerId
    if (!id) return "TBD"
    const name = teamId ? teamMap.get(teamId) : playerMap.get(playerId!)
    if (name) return name
    // While the maps are still loading we don't yet know the name — show a
    // neutral placeholder rather than a raw/truncated UUID. Once loaded, an
    // id that's genuinely absent from the (bounded) map degrades to a short id.
    return isLoading ? "…" : shortId(id)
  }

  return { resolve, isLoading }
}

function shortId(id: string): string {
  return id.length > 8 ? `${id.slice(0, 8)}…` : id
}
