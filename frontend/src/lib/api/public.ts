import type {
  PublicTournament,
  PublicMatchesResponse,
  PublicStandingsResponse,
  PublicMatchDetail,
} from "@/types/api/public"

// The public surface uses a plain fetch (NOT the authed axios client): it must
// send no credentials, no Authorization header, and never trigger the 401 token
// refresh/redirect interceptor. It works for anonymous visitors and runs on both
// server (generateMetadata / SSR) and client. A non-OK response throws PublicNotFound
// for 404 (the only meaningful state) or a generic error otherwise.
const BASE_URL = process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:8080"

export class PublicNotFoundError extends Error {
  constructor() {
    super("not found")
    this.name = "PublicNotFoundError"
  }
}

async function getJSON<T>(path: string): Promise<T> {
  const res = await fetch(`${BASE_URL}${path}`, {
    headers: { Accept: "application/json" },
    // Revalidate periodically; the backend also sets a short Cache-Control.
    next: { revalidate: 20 },
  })
  if (res.status === 404) throw new PublicNotFoundError()
  if (!res.ok) throw new Error(`public request failed: ${res.status}`)
  return res.json() as Promise<T>
}

const base = (orgSlug: string, tournamentSlug: string) =>
  `/api/v1/public/orgs/${encodeURIComponent(orgSlug)}/tournaments/${encodeURIComponent(tournamentSlug)}`

export const publicApi = {
  tournament: (orgSlug: string, tournamentSlug: string) =>
    getJSON<PublicTournament>(base(orgSlug, tournamentSlug)),

  matches: (orgSlug: string, tournamentSlug: string) =>
    getJSON<PublicMatchesResponse>(`${base(orgSlug, tournamentSlug)}/matches`),

  standings: (orgSlug: string, tournamentSlug: string) =>
    getJSON<PublicStandingsResponse>(`${base(orgSlug, tournamentSlug)}/standings`),

  match: (orgSlug: string, tournamentSlug: string, matchId: string) =>
    getJSON<PublicMatchDetail>(`${base(orgSlug, tournamentSlug)}/matches/${encodeURIComponent(matchId)}`),
}
