import { describe, it, expect, vi, beforeEach } from "vitest"
import { screen } from "@testing-library/react"
import { renderWithProviders } from "@/test/test-utils"
import RegistrationDashboardPage from "../[id]/registrations/page"
import { tournamentsApi } from "@/lib/api/tournaments"
import { registrationsApi } from "@/lib/api/registrations"
import type { Tournament } from "@/types/api/tournaments"

vi.mock("next/navigation", () => ({
  useParams: () => ({ orgSlug: "test-org", id: "t1" }),
  useRouter: () => ({ push: vi.fn(), replace: vi.fn() }),
  usePathname: () => "/test-org/tournaments/t1/registrations",
  useSearchParams: () => new URLSearchParams(),
}))

vi.mock("@/stores/auth.store", () => ({
  useAuthStore: (selector?: (s: unknown) => unknown) => {
    const state = {
      claims: {
        userId: "u1",
        email: "test@example.com",
        organizationId: "org1",
        role: "org_owner",
        exp: 9999999999,
      },
    }
    return selector ? selector(state) : state
  },
  selectRole: (s: { claims: { role: string } | null }) => s.claims?.role ?? null,
}))

vi.mock("@/lib/api/tournaments", () => ({
  tournamentsApi: {
    list: vi.fn(),
    getById: vi.fn(),
    create: vi.fn(),
    update: vi.fn(),
    delete: vi.fn(),
    getStandings: vi.fn(),
  },
}))

vi.mock("@/lib/api/registrations", () => ({
  registrationsApi: {
    list: vi.fn(),
    getById: vi.fn(),
    register: vi.fn(),
    update: vi.fn(),
    withdraw: vi.fn(),
  },
}))

vi.mock("@/lib/api/teams", () => ({
  teamsApi: { list: vi.fn().mockResolvedValue({ data: { teams: [], total: 0 } }) },
}))

vi.mock("@/lib/api/players", () => ({
  playersApi: { list: vi.fn().mockResolvedValue({ data: { players: [], total: 0 } }) },
}))

function makeTournament(overrides: Partial<Tournament> = {}): Tournament {
  return {
    id: "t1",
    organization_id: "org1",
    name: "Summer Knockout",
    slug: "summer-knockout",
    sport: "football",
    format: "knockout",
    status: "registration_open",
    participant_type: "team",
    description: null,
    banner_url: null,
    prize_pool: null,
    currency: "INR",
    max_participants: 16,
    min_participants: null,
    registration_opens_at: null,
    registration_closes_at: null,
    starts_at: null,
    ends_at: null,
    venue: null,
    city: null,
    country: null,
    rules: null,
    created_by: null,
    created_at: new Date().toISOString(),
    updated_at: new Date().toISOString(),
    registration_counts: {
      pending: 2,
      approved: 3,
      rejected: 0,
      withdrawn: 0,
      disqualified: 0,
      active: 5,
      total: 5,
    },
    ...overrides,
  }
}

beforeEach(() => {
  vi.clearAllMocks()
  vi.mocked(tournamentsApi.getById).mockResolvedValue({
    data: makeTournament(),
  } as never)
})

describe("RegistrationDashboardPage — query failure state (P2-04)", () => {
  it("shows an explicit error with retry instead of silently rendering an empty table", async () => {
    vi.mocked(registrationsApi.list).mockRejectedValue(new Error("network down"))

    renderWithProviders(<RegistrationDashboardPage />)

    expect(await screen.findByText("Failed to load registrations")).toBeInTheDocument()
    expect(screen.getByRole("button", { name: /retry/i })).toBeInTheDocument()
    // Must NOT show the empty state copy — an error is not "no registrations".
    expect(screen.queryByText("No registrations yet")).not.toBeInTheDocument()
  })

  it("keeps stat counts server-driven even when the list fails", async () => {
    vi.mocked(registrationsApi.list).mockRejectedValue(new Error("network down"))

    renderWithProviders(<RegistrationDashboardPage />)

    await screen.findByText("Failed to load registrations")
    // Counts come from tournament.registration_counts, not the failed list:
    // the Total stat shows 5, not a silent zero.
    expect(screen.getByText("Total")).toBeInTheDocument()
    expect(screen.getAllByText("5").length).toBeGreaterThan(0)
  })
})

describe("RegistrationDashboardPage — team name rendering (P1-05)", () => {
  it("renders the joined team name, never a UUID fragment", async () => {
    vi.mocked(registrationsApi.list).mockResolvedValue({
      data: {
        registrations: [
          {
            id: "r1",
            tournament_id: "t1",
            organization_id: "org1",
            team_id: "3f2a1b9c-aaaa-bbbb-cccc-000000000000",
            player_id: null,
            team_name: "Thunder Strikers",
            player_name: null,
            seed_number: null,
            status: "pending",
            registered_by: null,
            registered_at: new Date().toISOString(),
            approved_by: null,
            approved_at: null,
            notes: null,
            created_at: new Date().toISOString(),
            updated_at: new Date().toISOString(),
          },
        ],
        total: 1,
        limit: 50,
        offset: 0,
      },
    } as never)

    renderWithProviders(<RegistrationDashboardPage />)

    expect(await screen.findByText("Thunder Strikers")).toBeInTheDocument()
    expect(screen.queryByText(/3f2a1b9c/)).not.toBeInTheDocument()
    // Action labels are unique per participant for screen readers (P2-07).
    expect(
      screen.getByRole("button", { name: "Approve Thunder Strikers" }),
    ).toBeInTheDocument()
    expect(
      screen.getByRole("button", { name: "Reject Thunder Strikers" }),
    ).toBeInTheDocument()
  })
})
