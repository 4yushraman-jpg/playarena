import type { MatchStatus } from "@/types/api/matches"

/**
 * Match directory filter state, persisted in the URL so refreshes and shared
 * links restore the exact view. Mirrors the tournament directory convention.
 */
export interface MatchListState {
  search: string
  status: MatchStatus | "all"
  page: number
}

export const DEFAULT_MATCH_LIST_STATE: MatchListState = {
  search: "",
  status: "all",
  page: 0,
}

const VALID_STATUSES: ReadonlySet<string> = new Set([
  "scheduled",
  "live",
  "completed",
  "cancelled",
  "abandoned",
])

/** Parses directory state from URL search params, ignoring invalid values. */
export function parseMatchListState(params: URLSearchParams): MatchListState {
  const rawStatus = params.get("status") ?? "all"
  const rawPage = Number.parseInt(params.get("page") ?? "1", 10)
  return {
    search: params.get("q") ?? "",
    status: VALID_STATUSES.has(rawStatus) ? (rawStatus as MatchStatus) : "all",
    // URL is 1-based for humans; state is 0-based.
    page: Number.isFinite(rawPage) && rawPage > 1 ? rawPage - 1 : 0,
  }
}

/**
 * Serializes directory state to a query string. Defaults are omitted so the
 * canonical URL for the default view stays clean (`/matches`).
 */
export function serializeMatchListState(state: MatchListState): string {
  const params = new URLSearchParams()
  if (state.search) params.set("q", state.search)
  if (state.status !== "all") params.set("status", state.status)
  if (state.page > 0) params.set("page", String(state.page + 1))
  return params.toString()
}
