import { describe, it, expect, vi, beforeEach } from "vitest"
import { screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { renderWithProviders } from "@/test/test-utils"
import DashboardPage from "../page"
import { notificationKeys, tournamentKeys, matchKeys } from "@/lib/query-keys"
import { notificationsApi } from "@/lib/api/notifications"
import { tournamentsApi } from "@/lib/api/tournaments"
import { matchesApi } from "@/lib/api/matches"

// API mocks — default to a never-resolving promise so queries stay in loading
// state unless explicitly overridden in individual tests. This preserves the
// behavior of existing tests that seed cache data directly.
vi.mock("@/lib/api/notifications", () => ({
  notificationsApi: {
    list: vi.fn().mockImplementation(() => new Promise(() => {})),
  },
}))
vi.mock("@/lib/api/tournaments", () => ({
  tournamentsApi: {
    list: vi.fn().mockImplementation(() => new Promise(() => {})),
  },
}))
vi.mock("@/lib/api/matches", () => ({
  matchesApi: {
    list: vi.fn().mockImplementation(() => new Promise(() => {})),
  },
}))

// Next.js useParams mock
vi.mock("next/navigation", () => ({
  useParams: () => ({ orgSlug: "test-org" }),
  useRouter: () => ({ push: vi.fn(), replace: vi.fn() }),
  usePathname: () => "/test-org",
}))

// Auth store mock — org_owner role
vi.mock("@/stores/auth.store", () => ({
  useAuthStore: (selector?: (s: unknown) => unknown) => {
    const state = {
      claims: { userId: "u1", email: "test@example.com", organizationId: "org1", role: "org_owner", exp: 9999999999 },
      orgSlug: "test-org",
    }
    return selector ? selector(state) : state
  },
  selectRole: (s: { claims: { role: string } | null }) => s.claims?.role ?? null,
  selectUserId: (s: { claims: { userId: string } | null }) => s.claims?.userId ?? null,
}))

describe("DashboardPage", () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it("renders skeleton loading states while data is fetching", () => {
    renderWithProviders(<DashboardPage />)

    // Queries are pending — no data set yet
    expect(screen.queryByRole("article")).toBeNull()

    // The welcome heading should render immediately (no loading needed)
    expect(screen.getByText(/Welcome back/i)).toBeInTheDocument()
  })

  it("renders the welcome section with role badge", () => {
    renderWithProviders(<DashboardPage />)
    expect(screen.getByText(/Welcome back/i)).toBeInTheDocument()
    expect(screen.getByText("Owner")).toBeInTheDocument()
  })

  it("renders quick actions for org_owner role", () => {
    renderWithProviders(<DashboardPage />)
    expect(screen.getByText("New Tournament")).toBeInTheDocument()
    expect(screen.getByText("New Match")).toBeInTheDocument()
    expect(screen.getByText("Rankings")).toBeInTheDocument()
  })

  it("shows widget headings", () => {
    renderWithProviders(<DashboardPage />)
    // Use getAllByText because "Notifications" also appears in the nav badge label
    expect(screen.getAllByText("Notifications").length).toBeGreaterThanOrEqual(1)
    expect(screen.getByText("Tournaments")).toBeInTheDocument()
    expect(screen.getByText("Recent Matches")).toBeInTheDocument()
  })

  it("shows empty states after data loads with no items", async () => {
    const { client } = renderWithProviders(<DashboardPage />)

    // Seed empty data into the cache
    client.setQueryData(
      notificationKeys.list("test-org", { limit: 5, offset: 0 }),
      { notifications: [], total: 0, limit: 5, offset: 0 },
    )
    client.setQueryData(
      tournamentKeys.list("test-org", { limit: 5, status: "registration_open" }),
      { tournaments: [], total: 0, limit: 5, offset: 0 },
    )
    client.setQueryData(
      matchKeys.list("test-org", { limit: 5 }),
      { matches: [], total: 0, limit: 5, offset: 0 },
    )

    // Wait for the empty state text to appear
    await screen.findByText("No notifications yet")
    await screen.findByText("No active tournaments")
    await screen.findByText("No matches yet")
  })

  it("displays notification count badge when there are unreads", async () => {
    const { client } = renderWithProviders(<DashboardPage />)

    // useUnreadCount uses { limit: 50, offset: 0 } as the query key
    client.setQueryData(
      notificationKeys.list("test-org", { limit: 50, offset: 0 }),
      {
        notifications: [
          { id: "n1", read_at: null, event_type: "match_started", created_at: new Date().toISOString(), organization_id: "org1", user_id: "u1", outbox_id: "o1", channel: "in_app", entity_type: "match", entity_id: "m1", payload: {}, sent_at: null },
        ],
        total: 1,
        limit: 50,
        offset: 0,
      },
    )

    await screen.findByText("1 unread")
  })

  it("widget error states show a retry button that triggers refetch", async () => {
    const user = userEvent.setup()
    // Override the never-resolving defaults to reject immediately
    vi.mocked(notificationsApi.list)
      .mockRejectedValueOnce(new Error("network"))  // useUnreadCount call
      .mockRejectedValueOnce(new Error("network"))  // widget call
    vi.mocked(tournamentsApi.list).mockRejectedValueOnce(new Error("network"))
    vi.mocked(matchesApi.list).mockRejectedValueOnce(new Error("network"))

    renderWithProviders(<DashboardPage />)

    // All three widgets enter error state
    await waitFor(() => {
      expect(screen.getAllByText("Failed to load")).toHaveLength(3)
    })
    const retryButtons = screen.getAllByRole("button", { name: /retry/i })
    expect(retryButtons).toHaveLength(3)

    // Clicking a retry button triggers a refetch (mock returns pending again
    // to avoid cascading, so the widget returns to loading — not error)
    await user.click(retryButtons[0])
    // The notifications list is now called again (3rd call = refetch)
    await waitFor(() => {
      expect(vi.mocked(notificationsApi.list)).toHaveBeenCalledTimes(3)
    })
  })
})
