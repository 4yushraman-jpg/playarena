import { describe, it, expect, vi, beforeEach } from "vitest"
import { screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { renderWithProviders, makeTestQueryClient } from "@/test/test-utils"
import TeamsPage from "../page"
import { teamsApi } from "@/lib/api/teams"
import { teamKeys } from "@/lib/query-keys"
import type { Team } from "@/types/api/teams"

// ── Navigation mocks ───────────────────────────────────────────────────────────

vi.mock("next/navigation", () => ({
  useParams: () => ({ orgSlug: "test-org" }),
  useRouter: () => ({ push: vi.fn(), replace: vi.fn() }),
  usePathname: () => "/test-org/teams",
  useSearchParams: () => new URLSearchParams(),
}))

// ── Auth mocks ─────────────────────────────────────────────────────────────────

let mockRole = "org_owner"

vi.mock("@/stores/auth.store", () => ({
  useAuthStore: (selector?: (s: unknown) => unknown) => {
    const state = {
      claims: {
        userId: "u1",
        email: "test@example.com",
        organizationId: "org1",
        role: mockRole,
        exp: 9999999999,
      },
    }
    return selector ? selector(state) : state
  },
  selectRole: (s: { claims: { role: string } | null }) => s.claims?.role ?? null,
  selectUserId: (s: { claims: { userId: string } | null }) => s.claims?.userId ?? null,
}))

// ── API mocks ──────────────────────────────────────────────────────────────────

vi.mock("@/lib/api/teams", () => ({
  teamsApi: {
    list: vi.fn(),
    getById: vi.fn(),
    create: vi.fn(),
    update: vi.fn(),
    delete: vi.fn(),
    listMembers: vi.fn(),
    addMember: vi.fn(),
    removeMember: vi.fn(),
  },
}))

// ── Helpers ────────────────────────────────────────────────────────────────────

function makeTeam(overrides: Partial<Team> = {}): Team {
  return {
    id: "t1",
    organization_id: "org1",
    name: "Thunder Strikers",
    slug: "thunder-strikers",
    short_name: "TS",
    description: "A competitive team",
    logo_url: null,
    home_city: "Mumbai",
    home_venue: "Wankhede",
    founded_year: 2018,
    primary_color: "#FF6B00",
    secondary_color: "#FFFFFF",
    status: "active",
    created_at: new Date().toISOString(),
    updated_at: new Date().toISOString(),
    ...overrides,
  }
}

function seedTeams(teams: Team[], total?: number) {
  const client = makeTestQueryClient()
  client.setQueryData(teamKeys.list("test-org", { limit: 20, offset: 0 }), {
    teams,
    total: total ?? teams.length,
    limit: 20,
    offset: 0,
  })
  return renderWithProviders(<TeamsPage />, { client })
}

// ── Tests ──────────────────────────────────────────────────────────────────────

describe("TeamsPage — directory", () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockRole = "org_owner"
    vi.mocked(teamsApi.list).mockResolvedValue({
      data: { teams: [], total: 0, limit: 20, offset: 0 },
    } as never)
  })

  it("renders skeleton while loading", () => {
    renderWithProviders(<TeamsPage />)
    const skeletons = document.querySelectorAll(".animate-pulse")
    expect(skeletons.length).toBeGreaterThan(0)
  })

  it("shows first-time empty state with create CTA", async () => {
    seedTeams([])
    await screen.findByText("No teams yet")
    expect(screen.getByText(/Create your first team/i)).toBeInTheDocument()
    const createLinks = screen.getAllByRole("link", { name: /create first team|new team/i })
    expect(createLinks.length).toBeGreaterThan(0)
  })

  it("renders team rows with name and status", async () => {
    seedTeams([
      makeTeam({ id: "t1", name: "Thunder Strikers", status: "active" }),
      makeTeam({ id: "t2", name: "Iron Phoenix", status: "inactive" }),
    ])
    await screen.findByText("Thunder Strikers")
    expect(screen.getByText("Iron Phoenix")).toBeInTheDocument()
    expect(screen.getByText("Active")).toBeInTheDocument()
    expect(screen.getByText("Inactive")).toBeInTheDocument()
  })

  it("shows team count", async () => {
    seedTeams([makeTeam(), makeTeam({ id: "t2", name: "Iron Phoenix" })], 2)
    await screen.findByText("2 teams")
  })

  it("renders 'New team' link for org_owner", async () => {
    seedTeams([])
    await screen.findByText("No teams yet")
    const link = screen.getByRole("link", { name: /new team/i })
    expect(link).toHaveAttribute("href", "/test-org/teams/new")
  })

  it("links team name to profile page", async () => {
    seedTeams([makeTeam({ id: "t1", name: "Thunder Strikers" })])
    await screen.findByText("Thunder Strikers")
    const link = screen.getByRole("link", { name: /Thunder Strikers/i })
    expect(link).toHaveAttribute("href", "/test-org/teams/t1")
  })

  it("displays disbanded badge for disbanded teams", async () => {
    seedTeams([makeTeam({ id: "t1", status: "disbanded", name: "Old Giants" })])
    await screen.findByText("Old Giants")
    expect(screen.getByText("Disbanded")).toBeInTheDocument()
  })
})

// ── D-1: viewer role cannot create teams ─────────────────────────────────────

describe("TeamsPage — permission gates", () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(teamsApi.list).mockResolvedValue({
      data: { teams: [], total: 0, limit: 20, offset: 0 },
    } as never)
  })

  it("hides 'New team' button for viewer role", async () => {
    mockRole = "viewer"
    seedTeams([])
    await screen.findByText("No teams yet")
    expect(screen.queryByRole("link", { name: /new team/i })).toBeNull()
  })
})

// ── P1-4 replacement: the clear button test now actually tests the behavior ────

describe("TeamsPage — clear filters", () => {
  beforeEach(() => vi.clearAllMocks())

  it("shows Clear button when user types in search input", async () => {
    // Seed with a team so we render the table, not the empty state
    const client = makeTestQueryClient()
    client.setQueryData(teamKeys.list("test-org", { limit: 20, offset: 0 }), {
      teams: [makeTeam()],
      total: 1,
      limit: 20,
      offset: 0,
    })
    renderWithProviders(<TeamsPage />, { client })
    await screen.findByText("Thunder Strikers")

    // No active filter yet
    expect(screen.queryByRole("button", { name: /clear/i })).toBeNull()

    // Type into search — this sets localSearch which drives hasFilters
    const searchInput = screen.getByRole("textbox", { name: /search teams/i })
    await userEvent.type(searchInput, "t")

    // Clear button must now appear
    await waitFor(() => {
      expect(screen.getByRole("button", { name: /clear/i })).toBeInTheDocument()
    })
  })

  it("hides Clear button when filters are reset", async () => {
    const client = makeTestQueryClient()
    client.setQueryData(teamKeys.list("test-org", { limit: 20, offset: 0 }), {
      teams: [makeTeam()],
      total: 1,
      limit: 20,
      offset: 0,
    })
    renderWithProviders(<TeamsPage />, { client })
    await screen.findByText("Thunder Strikers")

    const searchInput = screen.getByRole("textbox", { name: /search teams/i })
    await userEvent.type(searchInput, "t")
    await waitFor(() => screen.getByRole("button", { name: /clear/i }))

    await userEvent.click(screen.getByRole("button", { name: /clear/i }))

    await waitFor(() => {
      expect(screen.queryByRole("button", { name: /clear/i })).toBeNull()
    })
  })
})
