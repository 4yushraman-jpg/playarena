/**
 * Regression + unit tests for team membership UI.
 *
 * Covers:
 *  - P0-1 regression: roster shows player_display_name, not raw UUIDs
 *  - P1-2 regression: AddMemberDialog disables players already on the team
 *  - add member happy path
 *  - remove member confirmation flow
 *  - permission-gated "Add member" button
 */

import { describe, it, expect, vi, beforeEach } from "vitest"
import { screen, waitFor, within } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { renderWithProviders, makeTestQueryClient } from "@/test/test-utils"
import { MembersSection } from "@/components/teams/members-section"
import { teamsApi } from "@/lib/api/teams"
import { playersApi } from "@/lib/api/players"
import { toast } from "sonner"
import { teamKeys, playerKeys } from "@/lib/query-keys"
import type { TeamMember } from "@/types/api/teams"
import type { Player } from "@/types/api/players"

// ── Toast mock ─────────────────────────────────────────────────────────────────

vi.mock("sonner", () => ({
  toast: {
    success: vi.fn(),
    error: vi.fn(),
  },
}))

// ── Navigation mocks ───────────────────────────────────────────────────────────

vi.mock("next/navigation", () => ({
  useParams: () => ({ orgSlug: "test-org" }),
  useRouter: () => ({ push: vi.fn(), replace: vi.fn() }),
  usePathname: () => "/test-org/teams/t1",
  useSearchParams: () => new URLSearchParams(),
}))

// ── Auth mocks ─────────────────────────────────────────────────────────────────

vi.mock("@/stores/auth.store", () => ({
  useAuthStore: (selector?: (s: unknown) => unknown) => {
    const state = {
      claims: { userId: "u1", email: "test@example.com", organizationId: "org1", role: "org_owner", exp: 9999999999 },
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

vi.mock("@/lib/api/players", () => ({
  playersApi: {
    list: vi.fn(),
    getById: vi.fn(),
    create: vi.fn(),
    update: vi.fn(),
    delete: vi.fn(),
  },
}))

// ── Helpers ────────────────────────────────────────────────────────────────────

function makeMember(overrides: Partial<TeamMember> = {}): TeamMember {
  return {
    id: "m1",
    team_id: "t1",
    player_id: "p1",
    organization_id: "org1",
    player_display_name: "Alice Johnson",
    role: "player",
    jersey_number: "10",
    status: "active",
    joined_at: new Date().toISOString(),
    left_at: null,
    notes: null,
    created_at: new Date().toISOString(),
    updated_at: new Date().toISOString(),
    ...overrides,
  }
}

function makePlayer(overrides: Partial<Player> = {}): Player {
  return {
    id: "p2",
    organization_id: "org1",
    user_id: null,
    display_name: "Bob Smith",
    jersey_number: "7",
    position: "Midfielder",
    height_cm: 175,
    weight_kg: 70,
    dominant_hand: "right",
    date_of_birth: "1998-05-20",
    nationality: "IN",
    bio: null,
    status: "active",
    avatar_url: null,
    created_at: new Date().toISOString(),
    updated_at: new Date().toISOString(),
    ...overrides,
  }
}

function renderMembers(
  members: TeamMember[],
  opts: { canManage?: boolean } = {},
) {
  const client = makeTestQueryClient()
  client.setQueryData(teamKeys.members("test-org", "t1"), {
    members,
  })
  return renderWithProviders(
    <MembersSection orgSlug="test-org" teamId="t1" canManage={opts.canManage ?? true} />,
    { client },
  )
}

// ── Tests ──────────────────────────────────────────────────────────────────────

describe("MembersSection — P0-1 regression: player names", () => {
  it("renders player_display_name, not the raw player_id UUID", async () => {
    renderMembers([makeMember({ player_id: "550e8400-e29b-41d4-a716-446655440000", player_display_name: "Alice Johnson" })])

    await screen.findByText("Alice Johnson")
    // The UUID must NOT appear anywhere visible
    expect(screen.queryByText("550e8400-e29b-41d4-a716-446655440000")).toBeNull()
  })

  it("renders all members with their display names", async () => {
    renderMembers([
      makeMember({ id: "m1", player_id: "p1", player_display_name: "Alice Johnson" }),
      makeMember({ id: "m2", player_id: "p2", player_display_name: "Carlos Diaz", jersey_number: "9" }),
    ])

    await screen.findByText("Alice Johnson")
    expect(screen.getByText("Carlos Diaz")).toBeInTheDocument()
  })

  it("uses player_display_name in the remove button aria-label", async () => {
    renderMembers([makeMember({ player_display_name: "Alice Johnson" })])

    await screen.findByText("Alice Johnson")
    expect(screen.getByRole("button", { name: /remove alice johnson from team/i })).toBeInTheDocument()
  })

  it("uses player_display_name in the remove confirm dialog", async () => {
    renderMembers([makeMember({ player_display_name: "Alice Johnson" })])

    await screen.findByText("Alice Johnson")
    await userEvent.click(screen.getByRole("button", { name: /remove alice johnson from team/i }))

    await waitFor(() => {
      expect(screen.getByRole("dialog")).toBeInTheDocument()
    })
    // dialog description contains the player name
    const dialog = screen.getByRole("dialog")
    expect(within(dialog).getByText(/alice johnson/i)).toBeInTheDocument()
  })

  it("shows member count correctly", async () => {
    renderMembers([
      makeMember({ id: "m1", player_display_name: "Alice Johnson" }),
      makeMember({ id: "m2", player_display_name: "Bob Smith" }),
    ])
    await screen.findByText("Alice Johnson")
    expect(screen.getByText("2 members")).toBeInTheDocument()
  })
})

describe("MembersSection — permissions", () => {
  it("hides Add member button when canManage is false", async () => {
    renderMembers([], { canManage: false })
    await waitFor(() => {
      expect(screen.queryByRole("button", { name: /add member/i })).toBeNull()
    })
  })

  it("shows Add member button when canManage is true", async () => {
    renderMembers([])
    // Empty state renders two "add member" targets: header button + empty state action
    await waitFor(() => {
      expect(screen.getAllByRole("button", { name: /add.*member/i }).length).toBeGreaterThan(0)
    })
  })
})

describe("MembersSection — remove member", () => {
  beforeEach(() => {
    vi.mocked(teamsApi.removeMember).mockResolvedValue({ data: undefined } as never)
  })

  it("calls removeMember API with the membership id on confirmation", async () => {
    renderMembers([makeMember({ id: "m1", player_display_name: "Alice Johnson" })])

    await screen.findByText("Alice Johnson")
    await userEvent.click(screen.getByRole("button", { name: /remove alice johnson from team/i }))

    await waitFor(() => screen.getByRole("dialog"))

    const confirmBtn = screen.getByRole("button", { name: /^remove$/i })
    await userEvent.click(confirmBtn)

    await waitFor(() => {
      expect(vi.mocked(teamsApi.removeMember)).toHaveBeenCalledWith(
        "test-org", "t1", "m1",
      )
    })
  })
})

describe("AddMemberDialog — P1-2 regression: duplicate guard", () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(playersApi.list).mockResolvedValue({
      data: {
        players: [
          makePlayer({ id: "p1", display_name: "Alice Johnson" }),
          makePlayer({ id: "p2", display_name: "Bob Smith" }),
        ],
        total: 2,
        limit: 20,
        offset: 0,
      },
    } as never)
  })

  it("marks already-added players as disabled with 'On team' label", async () => {
    // Alice (p1) is already on the team
    const client = makeTestQueryClient()
    client.setQueryData(teamKeys.members("test-org", "t1"), {
      members: [makeMember({ player_id: "p1", player_display_name: "Alice Johnson" })],
    })
    client.setQueryData(playerKeys.list("test-org", { status: "active", limit: 20 }), {
      players: [
        makePlayer({ id: "p1", display_name: "Alice Johnson" }),
        makePlayer({ id: "p2", display_name: "Bob Smith" }),
      ],
      total: 2, limit: 20, offset: 0,
    })

    renderWithProviders(
      <MembersSection orgSlug="test-org" teamId="t1" canManage={true} />,
      { client },
    )

    await screen.findByText("Alice Johnson")
    await userEvent.click(screen.getByRole("button", { name: /add.*member/i }))

    await waitFor(() => screen.getByRole("dialog"))

    // Wait for player list to load inside the dialog
    await waitFor(() => {
      expect(screen.getByText("Bob Smith")).toBeInTheDocument()
    })

    // Alice should be marked "On team"
    const dialog = screen.getByRole("dialog")
    const onTeamLabels = within(dialog).getAllByText("On team")
    expect(onTeamLabels.length).toBeGreaterThan(0)

    // Alice's button should be disabled
    const aliceButtons = within(dialog).getAllByRole("option")
    const aliceBtn = aliceButtons.find((b) => b.textContent?.includes("Alice Johnson"))
    expect(aliceBtn).toBeTruthy()
    expect(aliceBtn).toBeDisabled()

    // Bob should NOT be disabled
    const bobBtn = aliceButtons.find((b) => b.textContent?.includes("Bob Smith"))
    expect(bobBtn).toBeTruthy()
    expect(bobBtn).not.toBeDisabled()
  })
})

describe("AddMemberDialog — add member happy path", () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(playersApi.list).mockResolvedValue({
      data: {
        players: [makePlayer({ id: "p2", display_name: "Bob Smith" })],
        total: 1,
        limit: 20,
        offset: 0,
      },
    } as never)
    vi.mocked(teamsApi.addMember).mockResolvedValue({
      data: {
        id: "m-new",
        team_id: "t1",
        player_id: "p2",
        organization_id: "org1",
        player_display_name: "Bob Smith",
        role: "player",
        jersey_number: null,
        status: "active",
        joined_at: new Date().toISOString(),
        left_at: null,
        notes: null,
        created_at: new Date().toISOString(),
        updated_at: new Date().toISOString(),
      },
    } as never)
  })

  it("submits with the selected player_id and calls addMember", async () => {
    renderMembers([])

    // Click the header "Add member" button (the first one that matches)
    const addBtns = screen.getAllByRole("button", { name: /add.*member/i })
    await userEvent.click(addBtns[0])
    await waitFor(() => screen.getByRole("dialog"))
    await waitFor(() => screen.getByText("Bob Smith"))

    // Select Bob
    await userEvent.click(screen.getByRole("option", { name: /bob smith/i }))

    // Submit — scope to dialog to avoid ambiguity with the header button
    const dialog = screen.getByRole("dialog")
    await userEvent.click(within(dialog).getByRole("button", { name: /^add member$/i }))

    await waitFor(() => {
      expect(vi.mocked(teamsApi.addMember)).toHaveBeenCalledWith(
        "test-org",
        "t1",
        expect.objectContaining({ player_id: "p2" }),
      )
    })
  })
})

// ── C-3: addMember error state ────────────────────────────────────────────────

describe("AddMemberDialog — C-3: error state", () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(playersApi.list).mockResolvedValue({
      data: {
        players: [makePlayer({ id: "p2", display_name: "Bob Smith" })],
        total: 1,
        limit: 20,
        offset: 0,
      },
    } as never)
    vi.mocked(teamsApi.addMember).mockRejectedValue({
      response: { data: { error: "Player already on team" } },
    })
  })

  it("shows toast.error and keeps dialog open when addMember fails", async () => {
    renderMembers([])

    const addBtns = screen.getAllByRole("button", { name: /add.*member/i })
    await userEvent.click(addBtns[0])
    await waitFor(() => screen.getByRole("dialog"))
    await waitFor(() => screen.getByText("Bob Smith"))

    await userEvent.click(screen.getByRole("option", { name: /bob smith/i }))

    const dialog = screen.getByRole("dialog")
    await userEvent.click(within(dialog).getByRole("button", { name: /^add member$/i }))

    // toast.error should have been called
    await waitFor(() => {
      expect(vi.mocked(toast.error)).toHaveBeenCalled()
    })

    // Dialog must remain open — user can retry
    expect(screen.getByRole("dialog")).toBeInTheDocument()
  })
})
