import { describe, it, expect, vi, beforeEach } from "vitest"
import { screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { renderWithProviders, makeTestQueryClient } from "@/test/test-utils"
import { makeSlotResolver } from "../fixture-preview"
import { GenerateWizard } from "../generate-wizard"
import { FixtureGenerationPanel } from "../fixture-generation-panel"
import { matchKeys, tournamentKeys } from "@/lib/query-keys"
import type { Tournament } from "@/types/api/tournaments"
import type { GenerateResponse } from "@/types/api/fixtures"

vi.mock("@/lib/api/fixtures", () => ({
  fixturesApi: { generate: vi.fn(), resolveQualifiers: vi.fn(), generationInfo: vi.fn() },
}))
// Resolve participant ids to readable labels without team/player API wiring.
vi.mock("@/hooks/use-participant-names", () => ({
  useParticipantNames: () => ({
    resolve: (t: string | null, p: string | null) => (t || p ? `Team-${t || p}` : "TBD"),
    isLoading: false,
  }),
}))

import { fixturesApi } from "@/lib/api/fixtures"

function makeTournament(): Tournament {
  return {
    id: "t1",
    name: "Cup",
    format: "knockout",
    status: "registration_closed",
    participant_type: "team",
  } as Tournament
}

function knockoutPreview(seed?: number): GenerateResponse {
  const ids = ["a", "b", "c", "d"]
  return {
    dry_run: true,
    generated: 0,
    format: "knockout",
    seed_strategy: seed != null ? "random" : "registration",
    random_seed: seed,
    seed_order: ids,
    matches: [
      { round_number: 1, round_name: "Semi-final", match_number: 1, home: { kind: "participant", participant_id: "a" }, away: { kind: "participant", participant_id: "d" }, next_match_number: 3, next_match_slot: 1 },
      { round_number: 1, round_name: "Semi-final", match_number: 2, home: { kind: "participant", participant_id: "b" }, away: { kind: "participant", participant_id: "c" }, next_match_number: 3, next_match_slot: 2 },
      { round_number: 2, round_name: "Final", match_number: 3, home: { kind: "tbd" }, away: { kind: "tbd" } },
    ],
  }
}

beforeEach(() => {
  vi.clearAllMocks()
})

// ── slot resolver (pure) ────────────────────────────────────────────────────────

describe("makeSlotResolver", () => {
  const resolve = (t: string | null, p: string | null) => (t || p ? `Team-${t || p}` : "TBD")
  it("labels participant, qualifier, and TBD slots", () => {
    const r = makeSlotResolver(resolve, "team")
    expect(r({ kind: "participant", participant_id: "x" })).toBe("Team-x")
    expect(r({ kind: "qualifier", group: "A", rank: 1 })).toBe("Group A #1")
    expect(r({ kind: "tbd" })).toBe("TBD")
  })
  it("routes individual ids to the player slot", () => {
    const r = makeSlotResolver((t, p) => (t ? `T-${t}` : p ? `P-${p}` : "TBD"), "individual")
    expect(r({ kind: "participant", participant_id: "z" })).toBe("P-z")
  })
})

// ── wizard flow ──────────────────────────────────────────────────────────────────

describe("GenerateWizard", () => {
  it("walks format → settings → preview → confirm and echoes the random seed", async () => {
    const user = userEvent.setup()
    vi.mocked(fixturesApi.generate).mockImplementation((_org, _tid, body) => {
      if (body.dry_run) return Promise.resolve({ data: knockoutPreview(987654321) } as never)
      return Promise.resolve({ data: { ...knockoutPreview(987654321), dry_run: false, generated: 3, generated_at: "2026-06-14T00:00:00Z" } } as never)
    })

    renderWithProviders(
      <GenerateWizard open onOpenChange={() => {}} orgSlug="test-org" tournament={makeTournament()} />,
    )

    // Step 1 Format (knockout default) → Next
    await user.click(screen.getByRole("button", { name: "Next" }))
    // Step 2 Settings: choose Random, then Preview
    await user.click(screen.getByRole("button", { name: /Random/ }))
    await user.click(screen.getByRole("button", { name: "Preview" }))

    // Step 3 Structure summary appears (preview fetched)
    await screen.findByText("Structure")
    await waitFor(() => expect(screen.getByText("Matches")).toBeInTheDocument())
    // dry_run preview was requested
    expect(fixturesApi.generate).toHaveBeenCalledWith("test-org", "t1", expect.objectContaining({ dry_run: true, seed_strategy: "random" }))

    // Step 3 → 4 → 5
    await user.click(screen.getByRole("button", { name: "Next" }))
    await user.click(screen.getByRole("button", { name: "Next" }))
    // Confirm
    await user.click(screen.getByRole("button", { name: "Generate fixtures" }))

    await waitFor(() =>
      expect(fixturesApi.generate).toHaveBeenCalledWith(
        "test-org",
        "t1",
        expect.objectContaining({ dry_run: false, random_seed: 987654321, expected_participants: 4 }),
      ),
    )
  })

  it("blocks Preview advancing on error (stays on settings)", async () => {
    const user = userEvent.setup()
    vi.mocked(fixturesApi.generate).mockRejectedValue(new Error("boom"))
    renderWithProviders(
      <GenerateWizard open onOpenChange={() => {}} orgSlug="test-org" tournament={makeTournament()} />,
    )
    await user.click(screen.getByRole("button", { name: "Next" })) // to settings
    await user.click(screen.getByRole("button", { name: "Preview" }))
    // Preview button still shown (did not advance to Structure step).
    await waitFor(() => expect(screen.getByRole("button", { name: "Preview" })).toBeInTheDocument())
  })
})

// ── panel entry point ────────────────────────────────────────────────────────────

describe("FixtureGenerationPanel", () => {
  function seed(matches: unknown[] = [], generation?: unknown) {
    const client = makeTestQueryClient()
    client.setQueryData(matchKeys.list("test-org", { tournament_id: "t1", limit: 200 }), {
      matches, total: matches.length, limit: 200, offset: 0,
    })
    if (generation) client.setQueryData(tournamentKeys.generation("test-org", "t1"), generation)
    return client
  }

  it("offers Generate fixtures when registration is closed and no fixtures exist", async () => {
    const client = seed([])
    renderWithProviders(
      <FixtureGenerationPanel orgSlug="test-org" tournament={makeTournament()} canManage />, { client },
    )
    expect(await screen.findByRole("button", { name: /generate fixtures/i })).toBeInTheDocument()
  })

  it("hides Generate fixtures for a non-manager", async () => {
    const client = seed([])
    renderWithProviders(
      <FixtureGenerationPanel orgSlug="test-org" tournament={makeTournament()} canManage={false} />, { client },
    )
    // Panel renders nothing actionable → no generate button.
    expect(screen.queryByRole("button", { name: /generate fixtures/i })).toBeNull()
  })

  it("shows the reproducible generation record once fixtures exist", async () => {
    const client = seed(
      [{ id: "m1", group_label: null, round_number: 1, status: "scheduled", home_team_id: "a", away_team_id: "b", home_player_id: null, away_player_id: null }],
      { generated: true, format: "knockout", seed_strategy: "random", random_seed: 42, match_count: 7, generated_at: "2026-06-14T00:00:00Z" },
    )
    renderWithProviders(
      <FixtureGenerationPanel orgSlug="test-org" tournament={{ ...makeTournament(), status: "ongoing" }} canManage />, { client },
    )
    expect(await screen.findByText(/generation record/i)).toBeInTheDocument()
    expect(screen.getByText("42")).toBeInTheDocument()
  })
})
