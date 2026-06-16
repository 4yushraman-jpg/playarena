import { describe, it, expect, vi, beforeEach } from "vitest"
import { render, screen, within, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { renderWithProviders, makeTestQueryClient } from "@/test/test-utils"
import { PublicFixtures } from "../public-fixtures"
import { PublicStandings } from "../public-standings"
import { PublicBracket } from "../public-bracket"
import { PublicMatchDetail } from "../public-match-detail"
import { ShareTournament } from "@/components/tournaments/share-tournament"
import type { PublicMatch, PublicStandingsResponse, PublicMatchDetail as MatchDetail } from "@/types/api/public"
import type { Tournament } from "@/types/api/tournaments"

// next/navigation is used indirectly by some imports; stub it defensively.
vi.mock("next/navigation", () => ({
  usePathname: () => "/t/acme/cup",
  useRouter: () => ({ push: vi.fn(), replace: vi.fn(), refresh: vi.fn() }),
}))
vi.mock("@/lib/api/tournaments", () => ({
  tournamentsApi: { update: vi.fn(), getById: vi.fn(), list: vi.fn() },
}))
import { tournamentsApi } from "@/lib/api/tournaments"

const BASE = "/t/acme/cup"

function match(over: Partial<PublicMatch> = {}): PublicMatch {
  return {
    id: "m1",
    round_name: "Final",
    home: { kind: "participant", name: "Raiders" },
    away: { kind: "participant", name: "Kings" },
    status: "scheduled",
    home_score: 0,
    away_score: 0,
    is_walkover: false,
    ...over,
  }
}

beforeEach(() => vi.clearAllMocks())

describe("PublicFixtures", () => {
  it("renders matches and shows the score for completed ones", () => {
    render(<PublicFixtures basePath={BASE} matches={[match({ status: "completed", home_score: 30, away_score: 24, winner_name: "Raiders" })]} />)
    expect(screen.getByText("Raiders")).toBeInTheDocument()
    expect(screen.getByText(/30\s*–\s*24/)).toBeInTheDocument()
  })

  it("renders W/O for a walkover", () => {
    render(<PublicFixtures basePath={BASE} matches={[match({ status: "walkover", is_walkover: true, winner_name: "Raiders" })]} />)
    expect(screen.getByText("W/O")).toBeInTheDocument()
  })

  it("resultsOnly hides matches that are not concluded", () => {
    render(
      <PublicFixtures
        basePath={BASE}
        resultsOnly
        matches={[match({ id: "a", status: "scheduled" }), match({ id: "b", status: "completed", home_score: 1, away_score: 0, winner_name: "Raiders" })]}
      />,
    )
    // Only one (completed) row → exactly one matchup link.
    expect(screen.getAllByRole("link")).toHaveLength(1)
  })

  it("upcomingOnly hides concluded matches (so Fixtures ≠ Results)", () => {
    render(
      <PublicFixtures
        basePath={BASE}
        upcomingOnly
        matches={[
          match({ id: "a", status: "scheduled" }),
          match({ id: "b", status: "live" }),
          match({ id: "c", status: "completed", home_score: 1, away_score: 0, winner_name: "Raiders" }),
          match({ id: "d", status: "walkover", is_walkover: true, winner_name: "Raiders" }),
        ]}
      />,
    )
    // Only scheduled + live remain → two matchup links; concluded ones excluded.
    expect(screen.getAllByRole("link")).toHaveLength(2)
  })

  it("upcomingOnly shows the empty label once everything is concluded", () => {
    render(
      <PublicFixtures
        basePath={BASE}
        upcomingOnly
        emptyLabel="All matches have been played — see Results."
        matches={[match({ id: "a", status: "completed", home_score: 2, away_score: 1, winner_name: "Raiders" })]}
      />,
    )
    expect(screen.getByText("All matches have been played — see Results.")).toBeInTheDocument()
    expect(screen.queryByRole("link")).not.toBeInTheDocument()
  })
})

describe("PublicStandings", () => {
  function data(): PublicStandingsResponse {
    return {
      format: "league",
      status: "ongoing",
      point_system: { win_points: 3, draw_points: 1, loss_points: 0 },
      standings: [
        { position: 1, participant_name: "Raiders", played: 1, wins: 1, losses: 0, draws: 0, points: 3, score_for: 30, score_against: 20, score_difference: 10, disqualified: false },
        { position: 2, participant_name: "Kings", played: 1, wins: 0, losses: 1, draws: 0, points: 0, score_for: 20, score_against: 30, score_difference: -10, disqualified: true },
      ],
    }
  }
  it("renders a standings table with positions and flags disqualified rows", () => {
    render(<PublicStandings data={data()} />)
    const table = screen.getByRole("table")
    expect(within(table).getByText("Raiders")).toBeInTheDocument()
    expect(within(table).getByText("Disqualified")).toBeInTheDocument()
  })
})

describe("PublicBracket", () => {
  it("renders knockout rounds and excludes group-stage matches", () => {
    render(
      <PublicBracket
        basePath={BASE}
        matches={[
          match({ id: "g1", group_label: "A", round_number: 1, round_name: "Group A · Round 1" }),
          match({ id: "sf", round_number: 1, round_name: "Semi-final" }),
          match({ id: "f", round_number: 2, round_name: "Final" }),
        ]}
      />,
    )
    expect(screen.getByText("Semi-final")).toBeInTheDocument()
    expect(screen.getByText("Final")).toBeInTheDocument()
    // Group-stage section header should not appear in the bracket.
    expect(screen.queryByText(/Group A/)).toBeNull()
  })
})

describe("PublicMatchDetail", () => {
  it("shows scoreboard, winner and timeline", () => {
    const m: MatchDetail = {
      id: "m1",
      round_name: "Final",
      home: { kind: "participant", name: "Raiders" },
      away: { kind: "participant", name: "Kings" },
      status: "completed",
      home_score: 30,
      away_score: 24,
      is_walkover: false,
      winner_name: "Raiders",
      timeline: [{ sequence: 1, event_type: "raid_successful", actor_name: "Raiders" }],
    }
    render(<PublicMatchDetail match={m} />)
    expect(screen.getByText(/Winner: Raiders/)).toBeInTheDocument()
    expect(screen.getByText("Timeline")).toBeInTheDocument()
    expect(screen.getByText("Raid Successful")).toBeInTheDocument()
  })
})

describe("ShareTournament", () => {
  function tournament(over: Partial<Tournament> = {}): Tournament {
    return { id: "t1", slug: "cup", status: "ongoing", visibility: "unlisted", ...over } as Tournament
  }

  function renderShare(t: Tournament) {
    const client = makeTestQueryClient()
    return renderWithProviders(<ShareTournament orgSlug="acme" tournament={t} />, { client })
  }

  it("shows the public link when shareable and all three visibility options", () => {
    renderShare(tournament({ visibility: "unlisted" }))
    expect(screen.getByText(/\/t\/acme\/cup/)).toBeInTheDocument()
    expect(screen.getByRole("button", { name: /Private/ })).toBeInTheDocument()
    expect(screen.getByRole("button", { name: /Unlisted/ })).toBeInTheDocument()
    expect(screen.getByRole("button", { name: /Public/ })).toBeInTheDocument()
  })

  it("hides the public link and explains for a draft tournament", () => {
    renderShare(tournament({ status: "draft", visibility: "private" }))
    expect(screen.queryByText(/\/t\/acme\/cup/)).toBeNull()
    expect(screen.getByText(/Draft tournaments are never public/i)).toBeInTheDocument()
  })

  it("updates visibility when a different option is chosen", async () => {
    const user = userEvent.setup()
    vi.mocked(tournamentsApi.update).mockResolvedValue({ data: tournament({ visibility: "public" }) } as never)
    renderShare(tournament({ visibility: "unlisted" }))

    await user.click(screen.getByRole("button", { name: /Public/ }))
    await waitFor(() =>
      expect(tournamentsApi.update).toHaveBeenCalledWith("acme", "t1", { visibility: "public" }),
    )
  })
})
