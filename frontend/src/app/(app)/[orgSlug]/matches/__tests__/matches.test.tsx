import { describe, it, expect, vi, beforeEach } from "vitest"
import { screen, within, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { renderWithProviders, makeTestQueryClient } from "@/test/test-utils"
import MatchesPage from "../page"
import MatchDetailPage from "../[matchId]/page"
import { matchKeys, teamKeys, tournamentKeys } from "@/lib/query-keys"
import type { Match } from "@/types/api/matches"
import type { Team } from "@/types/api/teams"
import type { Tournament } from "@/types/api/tournaments"

// ── Navigation mocks ───────────────────────────────────────────────────────────

let mockMatchId = "m1"
vi.mock("next/navigation", () => ({
  useParams: () => ({ orgSlug: "test-org", matchId: mockMatchId }),
  useRouter: () => ({ push: vi.fn(), replace: vi.fn(), refresh: vi.fn() }),
  usePathname: () => "/test-org/matches",
  useSearchParams: () => new URLSearchParams(),
}))

// ── Auth mocks ─────────────────────────────────────────────────────────────────

let mockRole = "org_owner"
vi.mock("@/stores/auth.store", () => ({
  useAuthStore: (selector?: (s: unknown) => unknown) => {
    const state = { claims: { userId: "u1", role: mockRole, organizationId: "org1", exp: 9999999999 } }
    return selector ? selector(state) : state
  },
  selectRole: (s: { claims: { role: string } | null }) => s.claims?.role ?? null,
  selectUserId: (s: { claims: { userId: string } | null }) => s.claims?.userId ?? null,
}))

// ── API mocks (fallbacks for any query not pre-seeded) ─────────────────────────

vi.mock("@/lib/api/matches", () => ({
  matchesApi: { list: vi.fn(), getById: vi.fn(), create: vi.fn(), update: vi.fn(), delete: vi.fn(), walkover: vi.fn() },
}))
vi.mock("@/lib/api/teams", () => ({
  teamsApi: { list: vi.fn(), getById: vi.fn(), create: vi.fn(), update: vi.fn(), delete: vi.fn(), listMembers: vi.fn(), addMember: vi.fn(), removeMember: vi.fn() },
}))
vi.mock("@/lib/api/players", () => ({
  playersApi: { list: vi.fn(), getById: vi.fn(), create: vi.fn(), update: vi.fn(), delete: vi.fn() },
}))
vi.mock("@/lib/api/tournaments", () => ({
  tournamentsApi: { list: vi.fn(), getById: vi.fn(), create: vi.fn(), update: vi.fn(), delete: vi.fn(), getStandings: vi.fn() },
}))

import { matchesApi } from "@/lib/api/matches"
import { teamsApi } from "@/lib/api/teams"
import { playersApi } from "@/lib/api/players"
import { tournamentsApi } from "@/lib/api/tournaments"

// ── Factories ──────────────────────────────────────────────────────────────────

function makeMatch(overrides: Partial<Match> = {}): Match {
  return {
    id: "m1",
    organization_id: "org1",
    tournament_id: "tour1",
    round_number: 1,
    round_name: null,
    match_number: 2,
    home_team_id: "tm-raiders",
    away_team_id: "tm-kings",
    home_player_id: null,
    away_player_id: null,
    venue: "Court 1",
    scheduled_at: "2026-07-01T10:00:00Z",
    started_at: null,
    ended_at: null,
    status: "scheduled",
    winner_team_id: null,
    winner_player_id: null,
    is_walkover: false,
    home_score: 0,
    away_score: 0,
    notes: null,
    next_match_id: null,
    next_match_slot: null,
    group_label: null,
    created_at: "2026-06-01T00:00:00Z",
    updated_at: "2026-06-01T00:00:00Z",
    ...overrides,
  }
}

function makeTeam(id: string, name: string): Team {
  return {
    id, organization_id: "org1", name, slug: name.toLowerCase(), short_name: null,
    description: null, logo_url: null, home_city: null, home_venue: null,
    founded_year: null, primary_color: null, secondary_color: null, status: "active",
    created_at: "2026-01-01T00:00:00Z", updated_at: "2026-01-01T00:00:00Z",
  } as Team
}

function makeTournament(): Tournament {
  return { id: "tour1", name: "Summer Cup", participant_type: "team", status: "ongoing" } as Tournament
}

const TEAMS = [makeTeam("tm-raiders", "Raiders"), makeTeam("tm-kings", "Kings")]

function seedNames(client: ReturnType<typeof makeTestQueryClient>) {
  client.setQueryData(teamKeys.list("test-org", { limit: 200 }), {
    teams: TEAMS, total: TEAMS.length, limit: 200, offset: 0,
  })
  client.setQueryData(tournamentKeys.list("test-org", { limit: 100 }), {
    tournaments: [makeTournament()], total: 1, limit: 100, offset: 0,
  })
}

beforeEach(() => {
  vi.clearAllMocks()
  mockRole = "org_owner"
  mockMatchId = "m1"
  // Fallbacks so any non-seeded query resolves to empty instead of throwing.
  vi.mocked(matchesApi.list).mockResolvedValue({ data: { matches: [], total: 0, limit: 20, offset: 0 } } as never)
  vi.mocked(teamsApi.list).mockResolvedValue({ data: { teams: [], total: 0, limit: 200, offset: 0 } } as never)
  vi.mocked(playersApi.list).mockResolvedValue({ data: { players: [], total: 0, limit: 200, offset: 0 } } as never)
  vi.mocked(tournamentsApi.list).mockResolvedValue({ data: { tournaments: [], total: 0, limit: 100, offset: 0 } } as never)
})

// ── Matches directory ──────────────────────────────────────────────────────────

describe("MatchesPage — directory", () => {
  function seedMatches(matches: Match[], total?: number) {
    const client = makeTestQueryClient()
    client.setQueryData(matchKeys.list("test-org", { limit: 20, offset: 0, search: undefined, status: undefined }), {
      matches, total: total ?? matches.length, limit: 20, offset: 0,
    })
    seedNames(client)
    return renderWithProviders(<MatchesPage />, { client })
  }

  it("shows empty state when there are no matches", async () => {
    seedMatches([])
    await screen.findByText("No matches yet")
  })

  it("renders matchup with resolved participant names and status", async () => {
    seedMatches([makeMatch()])
    await screen.findByText(/Raiders/)
    const table = screen.getByRole("table", { name: "Matches" })
    expect(within(table).getByText(/Kings/)).toBeInTheDocument()
    // Target the status badge span, not the "Scheduled" date column header.
    expect(within(table).getByText("Scheduled", { selector: "span" })).toBeInTheDocument()
  })

  it("shows the final score only for completed matches", async () => {
    seedMatches([
      makeMatch({ id: "m1", status: "completed", home_score: 38, away_score: 31, winner_team_id: "tm-raiders" }),
    ])
    await screen.findByText(/Raiders/)
    expect(screen.getByText(/38\s*–\s*31/)).toBeInTheDocument()
  })

  it("links the matchup to the match detail page", async () => {
    seedMatches([makeMatch({ id: "m1" })])
    await screen.findByText(/Raiders/)
    const link = screen.getByRole("link", { name: /Raiders.*Kings/i })
    expect(link).toHaveAttribute("href", "/test-org/matches/m1")
  })

  it("labels the search box honestly (venue/round, not participant names)", async () => {
    seedMatches([makeMatch()])
    await screen.findByText(/Raiders/)
    // The backend only searches venue/round_name; the label must not imply
    // participant-name search.
    expect(screen.getByRole("textbox", { name: /search matches by venue or round/i })).toBeInTheDocument()
  })
})

// ── Match detail ───────────────────────────────────────────────────────────────

describe("MatchDetailPage", () => {
  function seedDetail(match: Match) {
    const client = makeTestQueryClient()
    client.setQueryData(matchKeys.detail("test-org", match.id), match)
    client.setQueryData(tournamentKeys.detail("test-org", match.tournament_id), makeTournament())
    client.setQueryData(teamKeys.list("test-org", { limit: 200 }), {
      teams: TEAMS, total: TEAMS.length, limit: 200, offset: 0,
    })
    return renderWithProviders(<MatchDetailPage />, { client })
  }

  it("renders the scoreboard and final result for a completed match", async () => {
    seedDetail(makeMatch({ status: "completed", home_score: 30, away_score: 24, winner_team_id: "tm-raiders" }))
    await screen.findAllByText(/Raiders/)
    expect(screen.getAllByText("Winner").length).toBeGreaterThan(0)
    expect(screen.getByText("Completed")).toBeInTheDocument()
  })

  it("exposes edit and cancel actions for a scheduled fixture (with permission)", async () => {
    seedDetail(makeMatch({ status: "scheduled" }))
    await screen.findAllByText(/Raiders/)
    const editLink = screen.getByRole("link", { name: /edit fixture/i })
    expect(editLink).toHaveAttribute("href", "/test-org/matches/m1/edit")
    expect(screen.getByRole("button", { name: /cancel fixture/i })).toBeInTheDocument()
  })

  it("hides fixture actions once the match is live or terminal", async () => {
    seedDetail(makeMatch({ status: "completed", home_score: 1, away_score: 0, winner_team_id: "tm-raiders" }))
    await screen.findAllByText(/Raiders/)
    expect(screen.queryByRole("link", { name: /edit fixture/i })).toBeNull()
    expect(screen.queryByRole("button", { name: /cancel fixture/i })).toBeNull()
  })

  it("hides fixture actions for a viewer even on a scheduled match", async () => {
    mockRole = "viewer"
    seedDetail(makeMatch({ status: "scheduled" }))
    await screen.findAllByText(/Raiders/)
    expect(screen.queryByRole("link", { name: /edit fixture/i })).toBeNull()
    expect(screen.queryByRole("button", { name: /cancel fixture/i })).toBeNull()
  })
})

// ── Walkover ─────────────────────────────────────────────────────────────────

describe("MatchDetailPage — walkover", () => {
  function seedDetail(match: Match) {
    const client = makeTestQueryClient()
    client.setQueryData(matchKeys.detail("test-org", match.id), match)
    client.setQueryData(tournamentKeys.detail("test-org", match.tournament_id), makeTournament())
    client.setQueryData(teamKeys.list("test-org", { limit: 200 }), {
      teams: TEAMS, total: TEAMS.length, limit: 200, offset: 0,
    })
    return renderWithProviders(<MatchDetailPage />, { client })
  }

  it("offers the walkover action for a scheduled match (with permission)", async () => {
    seedDetail(makeMatch({ status: "scheduled" }))
    await screen.findAllByText(/Raiders/)
    expect(screen.getByRole("button", { name: /award walkover/i })).toBeInTheDocument()
  })

  it("offers the walkover action for a live match (no-show mid-fixture)", async () => {
    seedDetail(makeMatch({ status: "live", started_at: "2026-07-01T10:05:00Z" }))
    await screen.findAllByText(/Raiders/)
    expect(screen.getByRole("button", { name: /award walkover/i })).toBeInTheDocument()
  })

  it("hides the walkover action for a viewer", async () => {
    mockRole = "viewer"
    seedDetail(makeMatch({ status: "scheduled" }))
    await screen.findAllByText(/Raiders/)
    expect(screen.queryByRole("button", { name: /award walkover/i })).toBeNull()
  })

  it("hides the walkover action once the match is terminal", async () => {
    seedDetail(makeMatch({ status: "walkover", is_walkover: true, winner_team_id: "tm-raiders" }))
    await screen.findAllByText(/Raiders/)
    expect(screen.queryByRole("button", { name: /award walkover/i })).toBeNull()
  })

  it("renders the walkover result with the winner highlighted (no misleading score)", async () => {
    seedDetail(makeMatch({ status: "walkover", is_walkover: true, winner_team_id: "tm-raiders" }))
    await screen.findAllByText(/Raiders/)
    // The W/O marker stands in for the 0-0 forfeit score.
    expect(screen.getByText("W/O")).toBeInTheDocument()
    // Walkover label appears (status badge + scoreboard marker).
    expect(screen.getAllByText("Walkover").length).toBeGreaterThan(0)
    expect(screen.getByText("Winner (walkover)")).toBeInTheDocument()
  })

  it("requires a winner and reason before the walkover can be submitted", async () => {
    const user = userEvent.setup()
    seedDetail(makeMatch({ status: "scheduled" }))
    await screen.findAllByText(/Raiders/)

    await user.click(screen.getByRole("button", { name: /award walkover/i }))

    const dialog = await screen.findByRole("dialog")
    const submit = within(dialog).getByRole("button", { name: /award walkover/i })
    // Disabled until a winner + reason are provided.
    expect(submit).toBeDisabled()

    await user.click(within(dialog).getByRole("button", { name: "Kings" }))
    expect(submit).toBeDisabled() // winner chosen, still needs a reason

    await user.type(within(dialog).getByLabelText(/reason/i), "Home team no-show")
    expect(submit).toBeEnabled()
  })

  it("calls the walkover API with the resolved winner and reason on confirm", async () => {
    const user = userEvent.setup()
    vi.mocked(matchesApi.walkover).mockResolvedValue({
      data: makeMatch({ status: "walkover", is_walkover: true, winner_team_id: "tm-kings" }),
    } as never)

    seedDetail(makeMatch({ status: "scheduled" }))
    await screen.findAllByText(/Raiders/)
    await user.click(screen.getByRole("button", { name: /award walkover/i }))

    const dialog = await screen.findByRole("dialog")
    await user.click(within(dialog).getByRole("button", { name: "Kings" }))
    await user.type(within(dialog).getByLabelText(/reason/i), "Home team no-show")
    await user.click(within(dialog).getByRole("button", { name: /award walkover/i }))

    await waitFor(() => {
      expect(matchesApi.walkover).toHaveBeenCalledWith("test-org", "m1", {
        winner: "away",
        reason: "Home team no-show",
      })
    })
  })
})

// ── Bracket linkage (FE-8B) ───────────────────────────────────────────────────

describe("MatchDetailPage — bracket linkage", () => {
  function seedDetail(match: Match, extra?: (c: ReturnType<typeof makeTestQueryClient>) => void) {
    const client = makeTestQueryClient()
    client.setQueryData(matchKeys.detail("test-org", match.id), match)
    client.setQueryData(tournamentKeys.detail("test-org", match.tournament_id), makeTournament())
    client.setQueryData(teamKeys.list("test-org", { limit: 200 }), {
      teams: TEAMS, total: TEAMS.length, limit: 200, offset: 0,
    })
    extra?.(client)
    return renderWithProviders(<MatchDetailPage />, { client })
  }

  it("renders TBD for an unresolved (bracket) participant slot", async () => {
    // A TBD downstream slot: no participants assigned yet.
    seedDetail(
      makeMatch({ id: "m1", home_team_id: null, away_team_id: null, round_name: "Final" }),
    )
    // Both sides resolve to the TBD placeholder.
    await waitFor(() => expect(screen.getAllByText("TBD").length).toBeGreaterThanOrEqual(2))
  })

  it("shows where the winner advances when the match is bracket-linked", async () => {
    mockMatchId = "m1"
    const successor = makeMatch({
      id: "m2",
      round_name: "Final",
      home_team_id: null,
      away_team_id: null,
    })
    seedDetail(
      makeMatch({ id: "m1", round_name: "Semi Final", next_match_id: "m2", next_match_slot: 1 }),
      (client) => {
        // Seed the successor so its round label resolves in the link.
        client.setQueryData(matchKeys.detail("test-org", "m2"), successor)
      },
    )

    const link = await screen.findByRole("link", { name: /winner advances to/i })
    expect(link).toHaveAttribute("href", "/test-org/matches/m2")
    expect(link).toHaveTextContent(/Final/)
  })

  it("shows no advances-to link for an unlinked match (e.g. a final)", async () => {
    seedDetail(makeMatch({ id: "m1", next_match_id: null, round_name: "Final" }))
    await screen.findAllByText(/Raiders/)
    expect(screen.queryByRole("link", { name: /winner advances to/i })).toBeNull()
  })
})
