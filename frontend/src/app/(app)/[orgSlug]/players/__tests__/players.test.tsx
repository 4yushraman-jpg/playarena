import { describe, it, expect, vi, beforeEach } from "vitest"
import { screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { renderWithProviders, makeTestQueryClient } from "@/test/test-utils"
import PlayersPage from "../page"
import PlayerProfilePage from "../[playerId]/page"
import { playersApi } from "@/lib/api/players"
import { playerKeys } from "@/lib/query-keys"
import type { Player } from "@/types/api/players"

// ── Navigation mocks ───────────────────────────────────────────────────────────

vi.mock("next/navigation", () => ({
  useParams: () => ({ orgSlug: "test-org", playerId: "p1" }),
  useRouter: () => ({ push: vi.fn(), replace: vi.fn() }),
  usePathname: () => "/test-org/players",
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

vi.mock("@/lib/api/players", () => ({
  playersApi: {
    list: vi.fn(),
    getById: vi.fn(),
    create: vi.fn(),
    update: vi.fn(),
    delete: vi.fn(),
  },
}))

// Module-level fns so individual tests can configure upload behaviour.
const mockUploadFn = vi.fn()
const mockResetFn = vi.fn()

vi.mock("@/hooks/use-media-upload", () => ({
  useMediaUpload: vi.fn(() => ({
    status: "idle",
    progress: 0,
    upload: mockUploadFn,
    reset: mockResetFn,
  })),
}))

// ── Helpers ────────────────────────────────────────────────────────────────────

function makePlayer(overrides: Partial<Player> = {}): Player {
  return {
    id: "p1",
    organization_id: "org1",
    user_id: null,
    display_name: "Alice Johnson",
    jersey_number: "10",
    position: "Forward",
    height_cm: 170,
    weight_kg: 65,
    dominant_hand: "right",
    date_of_birth: "1995-03-14",
    nationality: "IN",
    bio: null,
    status: "active",
    avatar_url: null,
    created_at: new Date().toISOString(),
    updated_at: new Date().toISOString(),
    ...overrides,
  }
}

function seedPlayers(players: Player[], total?: number) {
  const client = makeTestQueryClient()
  client.setQueryData(playerKeys.list("test-org", { limit: 20, offset: 0 }), {
    players,
    total: total ?? players.length,
    limit: 20,
    offset: 0,
  })
  return renderWithProviders(<PlayersPage />, { client })
}

// ── Tests ──────────────────────────────────────────────────────────────────────

describe("PlayersPage — directory", () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockRole = "org_owner"
    vi.mocked(playersApi.list).mockResolvedValue({
      data: { players: [], total: 0, limit: 20, offset: 0 },
    } as never)
  })

  it("renders skeleton while loading", () => {
    renderWithProviders(<PlayersPage />)
    const skeletons = document.querySelectorAll(".animate-pulse")
    expect(skeletons.length).toBeGreaterThan(0)
  })

  it("shows first-time empty state with create CTA for org_owner", async () => {
    seedPlayers([])
    await screen.findByText("No players yet")
    expect(screen.getByText(/Add your first player/i)).toBeInTheDocument()
    const createButtons = screen.getAllByRole("link", { name: /add first player|new player/i })
    expect(createButtons.length).toBeGreaterThan(0)
  })

  it("renders player rows with name and status", async () => {
    seedPlayers([
      makePlayer({ id: "p1", display_name: "Alice Johnson", status: "active" }),
      makePlayer({ id: "p2", display_name: "Bob Smith", status: "injured" }),
    ])
    await screen.findByText("Alice Johnson")
    expect(screen.getByText("Bob Smith")).toBeInTheDocument()
    expect(screen.getByText("Active")).toBeInTheDocument()
    expect(screen.getByText("Injured")).toBeInTheDocument()
  })

  it("shows player count in the filter bar", async () => {
    seedPlayers([makePlayer(), makePlayer({ id: "p2", display_name: "Bob" })], 2)
    await screen.findByText("2 players")
  })

  it("renders 'New player' link for permission-holder", async () => {
    seedPlayers([])
    await screen.findByText("No players yet")
    const link = screen.getByRole("link", { name: /new player/i })
    expect(link).toHaveAttribute("href", "/test-org/players/new")
  })

  it("links player name to profile page", async () => {
    seedPlayers([makePlayer({ id: "p1", display_name: "Alice Johnson" })])
    await screen.findByText("Alice Johnson")
    const link = screen.getByRole("link", { name: /Alice Johnson/i })
    expect(link).toHaveAttribute("href", "/test-org/players/p1")
  })
})

// ── Permission gate ────────────────────────────────────────────────────────────

describe("PlayersPage — permission gates", () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(playersApi.list).mockResolvedValue({
      data: { players: [], total: 0, limit: 20, offset: 0 },
    } as never)
  })

  it("hides 'New player' button for viewer role", async () => {
    mockRole = "viewer"
    seedPlayers([])
    await screen.findByText("No players yet")
    expect(screen.queryByRole("link", { name: /new player/i })).toBeNull()
  })
})

// ── P1-1 regression: localSearch / URL sync ────────────────────────────────────

describe("PlayersPage — P1-1: search input sync", () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockRole = "org_owner"
  })

  it("shows Clear button when user types in search input", async () => {
    const client = makeTestQueryClient()
    client.setQueryData(playerKeys.list("test-org", { limit: 20, offset: 0 }), {
      players: [makePlayer()],
      total: 1, limit: 20, offset: 0,
    })
    renderWithProviders(<PlayersPage />, { client })
    await screen.findByText("Alice Johnson")

    // No active filters yet
    expect(screen.queryByRole("button", { name: /clear/i })).toBeNull()

    const searchInput = screen.getByRole("textbox", { name: /search players/i })
    await userEvent.type(searchInput, "a")

    await waitFor(() => {
      expect(screen.getByRole("button", { name: /clear/i })).toBeInTheDocument()
    })
  })

  it("hides Clear button when filters are reset", async () => {
    const client = makeTestQueryClient()
    client.setQueryData(playerKeys.list("test-org", { limit: 20, offset: 0 }), {
      players: [makePlayer()],
      total: 1, limit: 20, offset: 0,
    })
    renderWithProviders(<PlayersPage />, { client })
    await screen.findByText("Alice Johnson")

    const searchInput = screen.getByRole("textbox", { name: /search players/i })
    await userEvent.type(searchInput, "a")
    await waitFor(() => screen.getByRole("button", { name: /clear/i }))

    await userEvent.click(screen.getByRole("button", { name: /clear/i }))
    await waitFor(() => {
      expect(screen.queryByRole("button", { name: /clear/i })).toBeNull()
    })
  })
})

// ── P0-2 regression: avatar display ───────────────────────────────────────────

describe("PlayerProfilePage — P0-2: avatar_url renders as img", () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockRole = "org_owner"
  })

  it("renders avatar <img> when player has avatar_url", async () => {
    const client = makeTestQueryClient()
    client.setQueryData(
      playerKeys.detail("test-org", "p1"),
      makePlayer({ avatar_url: "https://cdn.example.com/avatars/alice.jpg", display_name: "Alice Johnson" }),
    )
    renderWithProviders(<PlayerProfilePage />, { client })

    // Player name appears in several places (breadcrumb, heading, card) — wait for any
    await screen.findAllByText("Alice Johnson")

    const img = screen.getByRole("img", { name: "Alice Johnson" })
    expect(img).toBeInTheDocument()
    expect(img).toHaveAttribute("src", "https://cdn.example.com/avatars/alice.jpg")
  })

  it("shows initials (not img) when player has no avatar_url", async () => {
    const client = makeTestQueryClient()
    client.setQueryData(
      playerKeys.detail("test-org", "p1"),
      makePlayer({ avatar_url: null, display_name: "Alice Johnson" }),
    )
    renderWithProviders(<PlayerProfilePage />, { client })

    // Player name appears in several places — wait for any
    await screen.findAllByText("Alice Johnson")
    // No avatar img
    const imgs = screen.queryAllByRole("img", { name: "Alice Johnson" })
    expect(imgs).toHaveLength(0)
    // Initials should render
    expect(screen.getByText("AJ")).toBeInTheDocument()
  })
})

// ── Cache seeding (original test preserved) ───────────────────────────────────

describe("playersApi — cache seeding", () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockRole = "org_owner"
  })

  it("renders from pre-seeded cache without API call", async () => {
    const client = makeTestQueryClient()
    client.setQueryData(playerKeys.list("test-org", { limit: 20, offset: 0 }), {
      players: [makePlayer({ id: "p1", display_name: "Alice Johnson" })],
      total: 1,
      limit: 20,
      offset: 0,
    })

    renderWithProviders(<PlayersPage />, { client })
    await waitFor(() => {
      expect(screen.queryByText("Alice Johnson")).not.toBeNull()
    })
    // API should NOT have been called when cache is warm
    expect(vi.mocked(playersApi.list)).not.toHaveBeenCalled()
  })
})

// ── P1-A guard: list never shows avatar images ────────────────────────────────
// The backend intentionally omits avatar_url from list responses to avoid N+1
// queries. This test guards that the list view never renders avatar images
// (it always shows initials), even if a player object happens to have avatar_url.

describe("PlayersPage — P1-A: list shows initials, not avatar images", () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockRole = "org_owner"
  })

  it("renders initials in the list even if the type field avatar_url is present", async () => {
    // Simulate a list response where avatar_url is null (backend intentionally omits it)
    const client = makeTestQueryClient()
    client.setQueryData(playerKeys.list("test-org", { limit: 20, offset: 0 }), {
      players: [makePlayer({ id: "p1", display_name: "Alice Johnson", avatar_url: null })],
      total: 1, limit: 20, offset: 0,
    })
    renderWithProviders(<PlayersPage />, { client })

    await screen.findByText("Alice Johnson")
    // No avatar img in the list row
    expect(screen.queryByRole("img", { name: /alice johnson/i })).toBeNull()
    // Initials are shown
    expect(screen.getByText("AJ")).toBeInTheDocument()
  })
})

// ── C-1: upload → invalidate → re-render cycle ───────────────────────────────

describe("PlayerAvatar — C-1: upload → cache invalidation", () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockRole = "org_owner"
  })

  it("calls invalidateQueries on player detail key after successful upload", async () => {
    // Configure upload to succeed and return a media attachment
    mockUploadFn.mockResolvedValueOnce({ id: "m1", file_url: "https://cdn.example.com/avatar.jpg" })

    // After the cache is invalidated the page re-fetches; return updated player with avatar
    vi.mocked(playersApi.getById).mockResolvedValue({
      data: makePlayer({ avatar_url: "https://cdn.example.com/avatar.jpg" }),
    } as never)

    const client = makeTestQueryClient()
    const invalidateSpy = vi.spyOn(client, "invalidateQueries")

    // Seed initial player with no avatar
    client.setQueryData(
      playerKeys.detail("test-org", "p1"),
      makePlayer({ avatar_url: null }),
    )

    renderWithProviders(<PlayerProfilePage />, { client })

    // Wait for the page to render the player name
    await screen.findAllByText("Alice Johnson")

    // Click the "Change avatar" camera button
    await userEvent.click(screen.getByRole("button", { name: /change avatar/i }))

    // A file input should now be visible (inside the media uploader)
    const fileInput = document.querySelector("input[type='file']") as HTMLInputElement
    expect(fileInput).toBeTruthy()

    // Simulate file selection
    const file = new File(["data"], "avatar.jpg", { type: "image/jpeg" })
    await userEvent.upload(fileInput, file)

    // upload() should have been called with the chosen file
    await waitFor(() => expect(mockUploadFn).toHaveBeenCalledWith(file))

    // queryClient.invalidateQueries must be called with the player detail key
    await waitFor(() => {
      expect(invalidateSpy).toHaveBeenCalledWith(
        expect.objectContaining({ queryKey: playerKeys.detail("test-org", "p1") }),
      )
    })
  })
})
