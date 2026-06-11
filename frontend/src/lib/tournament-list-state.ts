import type { TournamentStatus } from "@/types/api/tournaments"

/**
 * Tournament directory filter state, persisted in the URL so refreshes and
 * shared links restore the exact view.
 */
export interface TournamentListState {
  search: string
  status: TournamentStatus | "all"
  page: number
}

export const DEFAULT_LIST_STATE: TournamentListState = {
  search: "",
  status: "all",
  page: 0,
}

const VALID_STATUSES: ReadonlySet<string> = new Set([
  "draft",
  "registration_open",
  "registration_closed",
  "ongoing",
  "completed",
  "cancelled",
])

/** Parses directory state from URL search params, ignoring invalid values. */
export function parseListState(params: URLSearchParams): TournamentListState {
  const rawStatus = params.get("status") ?? "all"
  const rawPage = Number.parseInt(params.get("page") ?? "1", 10)
  return {
    search: params.get("q") ?? "",
    status: VALID_STATUSES.has(rawStatus) ? (rawStatus as TournamentStatus) : "all",
    // URL is 1-based for humans; state is 0-based.
    page: Number.isFinite(rawPage) && rawPage > 1 ? rawPage - 1 : 0,
  }
}

/**
 * Serializes directory state to a query string. Defaults are omitted so the
 * canonical URL for the default view stays clean (`/tournaments`).
 */
export function serializeListState(state: TournamentListState): string {
  const params = new URLSearchParams()
  if (state.search) params.set("q", state.search)
  if (state.status !== "all") params.set("status", state.status)
  if (state.page > 0) params.set("page", String(state.page + 1))
  return params.toString()
}
