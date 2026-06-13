import { describe, it, expect, vi, beforeEach } from "vitest"
import { screen, within, fireEvent, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { renderWithProviders, makeTestQueryClient } from "@/test/test-utils"
import { LiveScorer } from "@/components/scorer/live-scorer"
import { matchKeys, teamKeys, tournamentKeys } from "@/lib/query-keys"
import type { Match, MatchStatus, LiveScore } from "@/types/api/matches"
import type { MatchEvent, MatchEventType } from "@/types/api/match-events"
import type { Team } from "@/types/api/teams"

// ── Mocks ────────────────────────────────────────────────────────────────────

const pushMock = vi.fn()
vi.mock("next/navigation", () => ({
  useRouter: () => ({ push: pushMock, replace: vi.fn(), refresh: vi.fn() }),
  useParams: () => ({ orgSlug: "test-org", matchId: "m1" }),
}))

vi.mock("@/lib/api/matches", () => ({
  matchesApi: { getById: vi.fn(), getScore: vi.fn(), list: vi.fn(), create: vi.fn(), update: vi.fn(), delete: vi.fn() },
}))
vi.mock("@/lib/api/match-events", () => ({ matchEventsApi: { list: vi.fn(), create: vi.fn() } }))

// Default role has NO scoring permission → read-only mode (preserves the
// FE-7BA read-only assertions). Scoring-mode tests set mockRole = "org_owner".
let mockRole = "viewer"
vi.mock("@/stores/auth.store", () => ({
  useAuthStore: (selector?: (s: unknown) => unknown) => {
    const state = { claims: { userId: "u1", role: mockRole, organizationId: "org1", exp: 9999999999 } }
    return selector ? selector(state) : state
  },
  selectRole: (s: { claims: { role: string } | null }) => s.claims?.role ?? null,
  selectUserId: (s: { claims: { userId: string } | null }) => s.claims?.userId ?? null,
}))
vi.mock("@/lib/api/teams", () => ({
  teamsApi: { list: vi.fn(), getById: vi.fn(), create: vi.fn(), update: vi.fn(), delete: vi.fn(), listMembers: vi.fn(), addMember: vi.fn(), removeMember: vi.fn() },
}))
vi.mock("@/lib/api/players", () => ({
  playersApi: { list: vi.fn(), getById: vi.fn(), create: vi.fn(), update: vi.fn(), delete: vi.fn() },
}))

import { matchesApi } from "@/lib/api/matches"
import { matchEventsApi } from "@/lib/api/match-events"
import { teamsApi } from "@/lib/api/teams"
import { playersApi } from "@/lib/api/players"

// ── Factories ──────────────────────────────────────────────────────────────────

function makeMatch(overrides: Partial<Match> = {}): Match {
  return {
    id: "m1", organization_id: "org1", tournament_id: "tour1",
    round_number: 1, round_name: null, match_number: 2,
    home_team_id: "tm-raiders", away_team_id: "tm-kings",
    home_player_id: null, away_player_id: null,
    venue: "Court 1", scheduled_at: "2026-07-01T10:00:00Z",
    started_at: null, ended_at: null, status: "live",
    winner_team_id: null, winner_player_id: null, is_walkover: false,
    home_score: 0, away_score: 0, notes: null,
    created_at: "2026-06-01T00:00:00Z", updated_at: "2026-06-01T00:00:00Z",
    ...overrides,
  }
}

function makeScore(home: number, away: number, status: MatchStatus): LiveScore {
  return {
    match_id: "m1", match_status: status, home_score: home, away_score: away,
    home_team_id: "tm-raiders", away_team_id: "tm-kings",
    home_player_id: null, away_player_id: null, is_walkover: false,
  }
}

let seq = 0
function ev(type: MatchEventType, opts: Partial<MatchEvent> = {}): MatchEvent {
  seq += 1
  return {
    id: opts.id ?? `e${seq}`, match_id: "m1", organization_id: "org1",
    sequence_number: opts.sequence_number ?? seq, event_type: type,
    team_id: opts.team_id ?? null, player_id: opts.player_id ?? null,
    period: opts.period ?? 1, clock_seconds: null, payload: opts.payload ?? {},
    recorded_by: "u1", recorded_at: "2026-07-01T10:05:00Z",
    cancels_event_id: opts.cancels_event_id ?? null, created_at: "2026-07-01T10:05:00Z",
  }
}

const TEAMS = [
  { id: "tm-raiders", organization_id: "org1", name: "Raiders", status: "active" },
  { id: "tm-kings", organization_id: "org1", name: "Kings", status: "active" },
] as unknown as Team[]

const EVENTS_PARAMS = { effective_only: false, limit: 500, offset: 0 }

function seedScorer({
  match,
  score,
  events = [],
}: {
  match: Match
  score?: LiveScore
  events?: MatchEvent[]
}) {
  const client = makeTestQueryClient()
  client.setQueryData(matchKeys.detail("test-org", "m1"), match)
  if (score) client.setQueryData(matchKeys.score("test-org", "m1"), score)
  client.setQueryData(matchKeys.events("test-org", "m1", EVENTS_PARAMS), {
    events, total: events.length, limit: 500, offset: 0, effective_only: false,
  })
  client.setQueryData(teamKeys.list("test-org", { limit: 200 }), {
    teams: TEAMS, total: TEAMS.length, limit: 200, offset: 0,
  })
  return renderWithProviders(<LiveScorer orgSlug="test-org" matchId="m1" />, { client })
}

beforeEach(() => {
  vi.clearAllMocks()
  seq = 0
  mockRole = "viewer"
  window.localStorage.clear()
  vi.mocked(matchesApi.getById).mockResolvedValue({ data: makeMatch() } as never)
  vi.mocked(matchesApi.getScore).mockResolvedValue({ data: makeScore(0, 0, "live") } as never)
  vi.mocked(matchEventsApi.list).mockResolvedValue({ data: { events: [], total: 0, limit: 500, offset: 0, effective_only: false } } as never)
  vi.mocked(matchEventsApi.create).mockReturnValue(new Promise(() => {}) as never)
  vi.mocked(teamsApi.list).mockResolvedValue({ data: { teams: TEAMS, total: 2, limit: 200, offset: 0 } } as never)
  vi.mocked(playersApi.list).mockResolvedValue({ data: { players: [], total: 0, limit: 200, offset: 0 } } as never)
})

// ── Loading / error ──────────────────────────────────────────────────────────

describe("LiveScorer — shell states", () => {
  it("shows a loading skeleton while the match loads", () => {
    const client = makeTestQueryClient()
    vi.mocked(matchesApi.getById).mockReturnValue(new Promise(() => {}) as never)
    renderWithProviders(<LiveScorer orgSlug="test-org" matchId="m1" />, { client })
    expect(screen.getByLabelText("Loading scorer")).toBeInTheDocument()
  })

  it("shows a not-found state and an exit button on error", async () => {
    const client = makeTestQueryClient()
    client.setQueryData(matchKeys.detail("test-org", "m1"), undefined)
    vi.mocked(matchesApi.getById).mockRejectedValue(new Error("404"))
    renderWithProviders(<LiveScorer orgSlug="test-org" matchId="m1" />, { client })
    await screen.findByText("Match not found")
    expect(screen.getByRole("button", { name: /back to match/i })).toBeInTheDocument()
  })
})

// ── Status-driven rendering ────────────────────────────────────────────────────

describe("LiveScorer — scheduled", () => {
  it("shows the start gate and no event history", async () => {
    seedScorer({ match: makeMatch({ status: "scheduled" }), score: makeScore(0, 0, "scheduled") })
    await screen.findByText(/hasn't started yet/i)
    expect(screen.queryByText("Event history")).toBeNull()
  })
})

describe("LiveScorer — live", () => {
  it("renders the authoritative score, live banner, sync banner and timeline", async () => {
    seedScorer({
      match: makeMatch({ status: "live" }),
      score: makeScore(7, 3, "live"),
      events: [
        ev("match_started"),
        ev("raid_successful", { id: "r1", team_id: "tm-raiders", payload: { points: 3 } }),
        ev("tackle_successful", { id: "t1", team_id: "tm-kings" }),
      ],
    })
    await screen.findByText("Live — read-only scoreboard")
    const board = screen.getByRole("region", { name: "Scoreboard" })
    expect(within(board).getByLabelText("Raiders score 7")).toBeInTheDocument()
    expect(within(board).getByLabelText("Kings score 3")).toBeInTheDocument()
    expect(screen.getByText(/Score from server/i)).toBeInTheDocument()
    expect(screen.getByText("Successful raid")).toBeInTheDocument()
    expect(screen.getByText("Successful tackle")).toBeInTheDocument()
  })

  it("marks a corrected event as struck-through with a Corrected tag", async () => {
    seedScorer({
      match: makeMatch({ status: "live" }),
      score: makeScore(0, 0, "live"),
      events: [
        ev("raid_successful", { id: "r1", team_id: "tm-raiders", payload: { points: 2 } }),
        ev("score_correction", { id: "c1", cancels_event_id: "r1" }),
      ],
    })
    await screen.findByText("Successful raid")
    expect(screen.getByText("Corrected")).toBeInTheDocument()
    expect(screen.getByText("Correction")).toBeInTheDocument()
  })

  it("surfaces a parity notice when the server score diverges from the event fold", async () => {
    // Authoritative says 7–3 but the (truncated) local events fold to 5–3.
    seedScorer({
      match: makeMatch({ status: "live" }),
      score: makeScore(7, 3, "live"),
      events: [
        ev("raid_successful", { id: "r1", team_id: "tm-raiders", payload: { points: 5 } }),
        ev("super_tackle", { id: "t1", team_id: "tm-kings" }),
      ],
    })
    await screen.findByText(/is authoritative/i)
  })

  it("shows the verified local fold (never a 0 snapshot) while the live server score is pending", async () => {
    // Authoritative score never resolves → must NOT fall back to the 0 snapshot.
    vi.mocked(matchesApi.getScore).mockReturnValue(new Promise(() => {}) as never)
    seedScorer({
      match: makeMatch({ status: "live" }),
      events: [
        ev("raid_successful", { team_id: "tm-raiders", payload: { points: 5 } }),
        ev("tackle_successful", { team_id: "tm-kings" }),
      ],
    })
    await screen.findByText(/computed locally from the event log/i)
    const board = screen.getByRole("region", { name: "Scoreboard" })
    expect(within(board).getByLabelText("Raiders score 5")).toBeInTheDocument()
    expect(within(board).getByLabelText("Kings score 1")).toBeInTheDocument()
  })

  it("falls back to the local fold and flags the failure when the live score request errors", async () => {
    vi.mocked(matchesApi.getScore).mockRejectedValue(new Error("network"))
    seedScorer({
      match: makeMatch({ status: "live" }),
      events: [ev("raid_successful", { team_id: "tm-raiders", payload: { points: 4 } })],
    })
    await screen.findByText(/couldn't reach the server/i)
    const board = screen.getByRole("region", { name: "Scoreboard" })
    expect(within(board).getByLabelText("Raiders score 4")).toBeInTheDocument()
  })

  it("does NOT show a parity notice when server score matches the fold", async () => {
    seedScorer({
      match: makeMatch({ status: "live" }),
      score: makeScore(3, 1, "live"),
      events: [
        ev("raid_successful", { id: "r1", team_id: "tm-raiders", payload: { points: 3 } }),
        ev("tackle_successful", { id: "t1", team_id: "tm-kings" }),
      ],
    })
    await screen.findByText("Live — read-only scoreboard")
    expect(screen.queryByText(/is authoritative/i)).toBeNull()
  })
})

describe("LiveScorer — terminal", () => {
  it("shows the final result banner and the winning side highlighted", async () => {
    seedScorer({
      match: makeMatch({ status: "completed", winner_team_id: "tm-raiders", home_score: 38, away_score: 31 }),
      score: makeScore(38, 31, "completed"),
    })
    await screen.findByText("Final result")
    const board = screen.getByRole("region", { name: "Scoreboard" })
    expect(within(board).getByLabelText("Raiders score 38")).toBeInTheDocument()
  })

  it("shows a cancelled banner for a cancelled match", async () => {
    seedScorer({ match: makeMatch({ status: "cancelled" }), score: makeScore(0, 0, "cancelled") })
    await screen.findByText("Match cancelled")
  })

  it("uses the stored snapshot for a completed match even before the score request resolves", async () => {
    vi.mocked(matchesApi.getScore).mockReturnValue(new Promise(() => {}) as never)
    seedScorer({
      match: makeMatch({ status: "completed", winner_team_id: "tm-raiders", home_score: 38, away_score: 31 }),
    })
    await screen.findByText("Final result")
    const board = screen.getByRole("region", { name: "Scoreboard" })
    expect(within(board).getByLabelText("Raiders score 38")).toBeInTheDocument()
    expect(within(board).getByLabelText("Kings score 31")).toBeInTheDocument()
  })
})

// ── Scheduled with start permission ─────────────────────────────────────────

describe("LiveScorer — scheduled (can start)", () => {
  it("shows a Start match button for a user with match.update", async () => {
    mockRole = "org_owner"
    seedScorer({ match: makeMatch({ status: "scheduled" }), score: makeScore(0, 0, "scheduled") })
    await screen.findByRole("button", { name: /start match/i })
  })
})

// ── Live scoring mode (scorer) ───────────────────────────────────────────────

describe("LiveScorer — live scoring", () => {
  it("renders scoring controls and the optimistic score from the event log", async () => {
    mockRole = "org_owner"
    seedScorer({
      match: makeMatch({ status: "live" }),
      score: makeScore(3, 1, "live"),
      events: [
        ev("raid_successful", { id: "r1", team_id: "tm-raiders", payload: { points: 3 } }),
        ev("tackle_successful", { id: "t1", team_id: "tm-kings" }),
      ],
    })
    await screen.findAllByText("Bonus +1") // scoring controls present (one per column)
    expect(screen.getAllByText("Bonus +1")).toHaveLength(2)
    expect(screen.getByRole("button", { name: /end and complete match/i })).toBeInTheDocument()
    // Read-only banner must NOT appear in scoring mode.
    expect(screen.queryByText("Live — read-only scoreboard")).toBeNull()
    const board = screen.getByRole("region", { name: "Scoreboard" })
    expect(within(board).getByLabelText("Raiders score 3")).toBeInTheDocument()
    expect(within(board).getByLabelText("Kings score 1")).toBeInTheDocument()
  })

  it("optimistically increments the score and shows it as unsynced when a point is tapped", async () => {
    mockRole = "org_owner"
    seedScorer({
      match: makeMatch({ status: "live" }),
      score: makeScore(0, 0, "live"),
      events: [],
    })
    const bonusButtons = await screen.findAllByRole("button", { name: /Raiders bonus point/i })
    await userEvent.click(bonusButtons[0])

    const board = screen.getByRole("region", { name: "Scoreboard" })
    expect(await within(board).findByLabelText("Raiders score 1")).toBeInTheDocument()
    expect(screen.getByText(/Syncing 1 event/i)).toBeInTheDocument()
    // The queue attempted exactly one POST for the one tap.
    expect(matchEventsApi.create).toHaveBeenCalledTimes(1)
  })

  it("blocks completion while events are unsynced", async () => {
    mockRole = "org_owner"
    seedScorer({ match: makeMatch({ status: "live" }), score: makeScore(0, 0, "live"), events: [] })
    const bonusButtons = await screen.findAllByRole("button", { name: /Raiders bonus point/i })
    await userEvent.click(bonusButtons[0])
    const complete = screen.getByRole("button", { name: /end and complete match/i })
    expect(complete).toBeDisabled()
    expect(screen.getByText(/Sync 1 event before completing/i)).toBeInTheDocument()
  })

  // P1-1: a double-fire of a scoring control must not double-score.
  it("debounces a double-tap into a single event", async () => {
    mockRole = "org_owner"
    seedScorer({ match: makeMatch({ status: "live" }), score: makeScore(0, 0, "live"), events: [] })
    const bonusButtons = await screen.findAllByRole("button", { name: /Raiders bonus point/i })
    // Two synchronous taps within the debounce window.
    fireEvent.click(bonusButtons[0])
    fireEvent.click(bonusButtons[0])
    const board = screen.getByRole("region", { name: "Scoreboard" })
    expect(await within(board).findByLabelText("Raiders score 1")).toBeInTheDocument()
    expect(screen.getByText(/Syncing 1 event/i)).toBeInTheDocument()
    expect(matchEventsApi.create).toHaveBeenCalledTimes(1)
  })

  // P1-2: completion must not be derivable from a still-loading server score.
  it("blocks completion until the authoritative score has settled", async () => {
    mockRole = "org_owner"
    vi.mocked(matchesApi.getScore).mockReturnValue(new Promise(() => {}) as never)
    seedScorer({ match: makeMatch({ status: "live" }), events: [] })
    await screen.findByRole("button", { name: /end and complete match/i })
    expect(screen.getByRole("button", { name: /end and complete match/i })).toBeDisabled()
    expect(screen.getByText(/Loading the authoritative score/i)).toBeInTheDocument()
  })
})

// ── FE-7BC match operations ──────────────────────────────────────────────────

describe("LiveScorer — completion", () => {
  it("completes with the winner derived from the authoritative score and refreshes standings", async () => {
    mockRole = "org_owner"
    vi.mocked(matchesApi.update).mockResolvedValue({
      data: makeMatch({ status: "completed", winner_team_id: "tm-raiders", home_score: 31, away_score: 28 }),
    } as never)
    const utils = seedScorer({
      match: makeMatch({ status: "live" }),
      score: makeScore(31, 28, "live"),
      events: [],
    })
    const invalidateSpy = vi.spyOn(utils.client, "invalidateQueries")

    const endBtn = await screen.findByRole("button", { name: /end and complete match/i })
    await waitFor(() => expect(endBtn).toBeEnabled())
    await userEvent.click(endBtn)

    await screen.findByText(/Raiders win 31/i)
    await userEvent.click(screen.getByRole("button", { name: /^complete match$/i }))

    expect(matchesApi.update).toHaveBeenCalledWith("test-org", "m1", {
      status: "completed",
      winner_team_id: "tm-raiders",
    })
    await waitFor(() =>
      expect(invalidateSpy).toHaveBeenCalledWith({
        queryKey: tournamentKeys.standings("test-org", "tour1"),
      }),
    )
  })

  it("guards against double completion — the terminal action fires once", async () => {
    mockRole = "org_owner"
    vi.mocked(matchesApi.update).mockResolvedValue({
      data: makeMatch({ status: "completed", winner_team_id: "tm-raiders", home_score: 5, away_score: 2 }),
    } as never)
    seedScorer({ match: makeMatch({ status: "live" }), score: makeScore(5, 2, "live"), events: [] })
    const endBtn = await screen.findByRole("button", { name: /end and complete match/i })
    await waitFor(() => expect(endBtn).toBeEnabled())
    await userEvent.click(endBtn)
    const confirm = await screen.findByRole("button", { name: /^complete match$/i })
    fireEvent.click(confirm)
    fireEvent.click(confirm)
    await waitFor(() => expect(matchesApi.update).toHaveBeenCalledTimes(1))
  })

  it("abandons a match (no winner) via the gated abandon flow", async () => {
    mockRole = "org_owner"
    vi.mocked(matchesApi.update).mockResolvedValue({
      data: makeMatch({ status: "abandoned" }),
    } as never)
    seedScorer({ match: makeMatch({ status: "live" }), score: makeScore(0, 0, "live"), events: [] })

    await userEvent.click(await screen.findByRole("button", { name: /^abandon match$/i }))
    const dialogConfirm = screen.getAllByRole("button", { name: /abandon match/i }).at(-1)!
    await userEvent.click(dialogConfirm)

    expect(matchesApi.update).toHaveBeenCalledWith("test-org", "m1", { status: "abandoned" })
  })
})

describe("LiveScorer — periods & more events", () => {
  it("emits a half_started event on starting the next half", async () => {
    mockRole = "org_owner"
    seedScorer({ match: makeMatch({ status: "live" }), score: makeScore(0, 0, "live"), events: [] })
    await userEvent.click(await screen.findByRole("button", { name: /start half 2/i }))
    await waitFor(() =>
      expect(matchEventsApi.create).toHaveBeenCalledWith(
        "test-org",
        "m1",
        expect.objectContaining({ event_type: "half_started" }),
      ),
    )
  })

  it("records a timeout from the More sheet", async () => {
    mockRole = "org_owner"
    seedScorer({ match: makeMatch({ status: "live" }), score: makeScore(0, 0, "live"), events: [] })
    await userEvent.click(await screen.findByRole("button", { name: /^more$/i }))
    await userEvent.click(await screen.findByRole("button", { name: /^timeout$/i }))
    await waitFor(() =>
      expect(matchEventsApi.create).toHaveBeenCalledWith(
        "test-org",
        "m1",
        expect.objectContaining({ event_type: "timeout_called" }),
      ),
    )
  })

  it("records a penalty to a chosen side from the More sheet", async () => {
    mockRole = "org_owner"
    seedScorer({ match: makeMatch({ status: "live" }), score: makeScore(0, 0, "live"), events: [] })
    await userEvent.click(await screen.findByRole("button", { name: /^more$/i }))
    await userEvent.click(await screen.findByRole("button", { name: /award Raiders/i }))
    await waitFor(() =>
      expect(matchEventsApi.create).toHaveBeenCalledWith(
        "test-org",
        "m1",
        expect.objectContaining({ event_type: "penalty_awarded", team_id: "tm-raiders" }),
      ),
    )
  })
})

describe("LiveScorer — accessibility", () => {
  it("announces the score via a polite live region in scoring mode", async () => {
    mockRole = "org_owner"
    seedScorer({
      match: makeMatch({ status: "live" }),
      score: makeScore(2, 0, "live"),
      events: [ev("raid_successful", { team_id: "tm-raiders", payload: { points: 2 } })],
    })
    const live = await screen.findByText("Raiders 2, Kings 0")
    expect(live).toHaveAttribute("aria-live", "polite")
  })
})

describe("LiveScorer — concurrent scorer awareness", () => {
  it("warns when the event log contains events from another account", async () => {
    mockRole = "org_owner"
    seedScorer({
      match: makeMatch({ status: "live" }),
      score: makeScore(2, 0, "live"),
      events: [
        { ...ev("raid_successful", { team_id: "tm-raiders", payload: { points: 2 } }), recorded_by: "other-scorer" },
      ],
    })
    await screen.findByText(/Another scorer is recording this match/i)
  })
})

// ── Interaction ────────────────────────────────────────────────────────────────

describe("LiveScorer — exit", () => {
  it("navigates back to the match detail on exit", async () => {
    seedScorer({ match: makeMatch({ status: "live" }), score: makeScore(0, 0, "live") })
    await screen.findByText("Live — read-only scoreboard")
    await userEvent.click(screen.getByRole("button", { name: /exit scorer/i }))
    expect(pushMock).toHaveBeenCalledWith("/test-org/matches/m1")
  })
})
