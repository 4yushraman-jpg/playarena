/**
 * Tests for the team detail page (TeamProfilePage).
 *
 * Covers:
 *  - C-2: disband confirmation flow — user clicks Disband, confirms, API is called
 *  - D-2: TeamLogo primaryColor regression — inline style applied to single div
 */

import { describe, it, expect, vi, beforeEach } from "vitest"
import { screen, waitFor, within } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { renderWithProviders, makeTestQueryClient } from "@/test/test-utils"
import TeamProfilePage from "../[teamId]/page"
import { TeamLogo } from "@/components/teams/team-logo"
import { teamsApi } from "@/lib/api/teams"
import { teamKeys } from "@/lib/query-keys"
import type { Team } from "@/types/api/teams"

// ── Navigation mocks ───────────────────────────────────────────────────────────

const mockPush = vi.fn()

vi.mock("next/navigation", () => ({
  useParams: () => ({ orgSlug: "test-org", teamId: "t1" }),
  useRouter: () => ({ push: mockPush, replace: vi.fn() }),
  usePathname: () => "/test-org/teams/t1",
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

// ── Media upload mock ──────────────────────────────────────────────────────────

vi.mock("@/hooks/use-media-upload", () => ({
  useMediaUpload: vi.fn(() => ({
    status: "idle",
    progress: 0,
    upload: vi.fn(),
    reset: vi.fn(),
  })),
}))

// ── Helpers ────────────────────────────────────────────────────────────────────

function makeTeam(overrides: Partial<Team> = {}): Team {
  return {
    id: "t1",
    organization_id: "org1",
    name: "Thunder Strikers",
    slug: "thunder-strikers",
    short_name: "TS",
    description: null,
    logo_url: null,
    home_city: "Mumbai",
    home_venue: null,
    founded_year: 2018,
    primary_color: "#FF6B00",
    secondary_color: null,
    status: "active",
    created_at: new Date().toISOString(),
    updated_at: new Date().toISOString(),
    ...overrides,
  }
}

function renderTeam(team: Team, opts: { canDelete?: boolean } = {}) {
  if (opts.canDelete !== undefined) {
    mockRole = opts.canDelete ? "org_owner" : "viewer"
  }
  const client = makeTestQueryClient()
  client.setQueryData(teamKeys.detail("test-org", "t1"), team)
  // Stub member list so MembersSection renders without errors
  client.setQueryData(teamKeys.members("test-org", "t1"), { members: [] })
  return renderWithProviders(<TeamProfilePage />, { client })
}

// ── C-2: Disband confirmation flow ────────────────────────────────────────────

describe("TeamProfilePage — C-2: disband confirmation", () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockRole = "org_owner"
    vi.mocked(teamsApi.delete).mockResolvedValue({ data: undefined } as never)
    vi.mocked(teamsApi.listMembers).mockResolvedValue({
      data: { members: [] },
    } as never)
  })

  it("shows disband confirm dialog when Disband button is clicked", async () => {
    renderTeam(makeTeam())

    await screen.findAllByText("Thunder Strikers")

    await userEvent.click(screen.getByRole("button", { name: /disband/i }))

    await waitFor(() => {
      expect(screen.getByRole("dialog")).toBeInTheDocument()
    })

    const dialog = screen.getByRole("dialog")
    expect(within(dialog).getByText(/disband team/i)).toBeInTheDocument()
    expect(within(dialog).getByText(/thunder strikers/i)).toBeInTheDocument()
  })

  it("calls teamsApi.delete and navigates away on confirm", async () => {
    renderTeam(makeTeam())

    await screen.findAllByText("Thunder Strikers")

    await userEvent.click(screen.getByRole("button", { name: /disband/i }))
    await waitFor(() => screen.getByRole("dialog"))

    const dialog = screen.getByRole("dialog")
    await userEvent.click(within(dialog).getByRole("button", { name: /^disband$/i }))

    await waitFor(() => {
      expect(vi.mocked(teamsApi.delete)).toHaveBeenCalledWith("test-org", "t1")
    })
    await waitFor(() => {
      expect(mockPush).toHaveBeenCalledWith("/test-org/teams")
    })
  })

  it("does not call delete when cancel is clicked", async () => {
    renderTeam(makeTeam())

    await screen.findAllByText("Thunder Strikers")

    await userEvent.click(screen.getByRole("button", { name: /disband/i }))
    await waitFor(() => screen.getByRole("dialog"))

    const dialog = screen.getByRole("dialog")
    await userEvent.click(within(dialog).getByRole("button", { name: /cancel/i }))

    await waitFor(() => {
      expect(screen.queryByRole("dialog")).toBeNull()
    })
    expect(vi.mocked(teamsApi.delete)).not.toHaveBeenCalled()
  })
})

// ── D-2: TeamLogo primaryColor regression ─────────────────────────────────────
// After the P1-B fix, TeamLogo applies primaryColor as an inline style on a
// single div (no inner bg-muted override div). This test guards that fix.

describe("TeamLogo — D-2: primaryColor applied to single display div", () => {
  it("applies primaryColor as inline style when no logoUrl is given", () => {
    const { container } = renderWithProviders(
      <TeamLogo
        orgSlug="test-org"
        teamId="t1"
        logoUrl={null}
        teamName="Thunder Strikers"
        primaryColor="#FF6B00"
        size="md"
        canUpload={false}
      />,
    )

    // The display div (the one showing initials) should carry the inline style
    const styled = container.querySelector<HTMLElement>("[style*='background-color']")
    expect(styled).not.toBeNull()
    expect(styled?.style.backgroundColor).toBeTruthy()

    // There must NOT be a nested div overriding with bg-muted class that
    // would cancel the primaryColor (the old bug: inner bg-muted child div)
    const innerMutedDivs = container.querySelectorAll(
      "[style*='background-color'] > div.bg-muted",
    )
    expect(innerMutedDivs.length).toBe(0)
  })

  it("renders initials inside the styled div (not a nested fallback div)", () => {
    const { container } = renderWithProviders(
      <TeamLogo
        orgSlug="test-org"
        teamId="t1"
        logoUrl={null}
        teamName="Thunder Strikers"
        primaryColor="#FF6B00"
        size="md"
        canUpload={false}
      />,
    )

    // Initials should appear directly (not inside a nested div)
    expect(screen.getByText("TS")).toBeInTheDocument()

    // The styled div should contain the initials as direct text content
    const styled = container.querySelector<HTMLElement>("[style*='background-color']")
    expect(styled?.textContent).toContain("TS")
  })

  it("does not apply inline style when primaryColor is not provided", () => {
    const { container } = renderWithProviders(
      <TeamLogo
        orgSlug="test-org"
        teamId="t1"
        logoUrl={null}
        teamName="Thunder Strikers"
        size="md"
        canUpload={false}
      />,
    )

    const styled = container.querySelector("[style*='background-color']")
    expect(styled).toBeNull()
  })
})
