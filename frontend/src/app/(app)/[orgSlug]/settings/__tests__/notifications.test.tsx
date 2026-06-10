import { describe, it, expect, vi, beforeEach } from "vitest"
import { screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { renderWithProviders, makeTestQueryClient } from "@/test/test-utils"
import NotificationPreferencesPage from "../notifications/page"
import { notificationsApi } from "@/lib/api/notifications"
import { notificationKeys } from "@/lib/query-keys"
import type { NotificationPreference } from "@/types/api/notifications"

vi.mock("next/navigation", () => ({
  useParams: () => ({ orgSlug: "test-org" }),
  useRouter: () => ({ push: vi.fn() }),
  usePathname: () => "/test-org/settings/notifications",
}))

vi.mock("@/stores/auth.store", () => ({
  useAuthStore: (selector?: (s: unknown) => unknown) => {
    const state = {
      claims: { userId: "u1", email: "test@example.com", organizationId: "org1", role: "org_owner", exp: 9999999999 },
    }
    return selector ? selector(state) : state
  },
  selectUserId: (s: { claims: { userId: string } | null }) => s.claims?.userId ?? null,
}))

vi.mock("@/lib/api/notifications", () => ({
  notificationsApi: {
    getPreferences: vi.fn().mockResolvedValue({ data: { preferences: [] } }),
    updatePreference: vi.fn().mockResolvedValue({ data: {} }),
    list: vi.fn().mockResolvedValue({ data: { notifications: [], total: 0, limit: 50, offset: 0 } }),
  },
}))

const MOCK_PREFERENCES: NotificationPreference[] = [
  { id: "p1", organization_id: "org1", user_id: "u1", event_type: "match_started", channel: "in_app", enabled: true, updated_at: "2024-01-01T00:00:00Z" },
  { id: "p2", organization_id: "org1", user_id: "u1", event_type: "match_started", channel: "email", enabled: false, updated_at: "2024-01-01T00:00:00Z" },
]

// Pre-seed cache so query resolves immediately without network
function setup(prefs: NotificationPreference[] = MOCK_PREFERENCES) {
  const client = makeTestQueryClient()
  client.setQueryData(notificationKeys.preferences("test-org"), { preferences: prefs })
  return renderWithProviders(<NotificationPreferencesPage />, { client })
}

describe("NotificationPreferencesPage", () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it("shows skeleton while loading", () => {
    renderWithProviders(<NotificationPreferencesPage />)
    const skeletons = document.querySelectorAll(".animate-pulse")
    expect(skeletons.length).toBeGreaterThan(0)
  })

  it("renders event type groups after data loads", async () => {
    setup()
    await screen.findByText("Matches")
    await screen.findByText("Tournaments")
    await screen.findByText("Registrations")
  })

  it("renders column headers for in-app and email channels", async () => {
    setup()
    await screen.findByText("In-app")
    await screen.findByText("Email")
  })

  it("reflects stored preference state in toggles", async () => {
    setup()
    await screen.findByText("Match Started")

    const matchStartedInApp = screen.getByLabelText("Match Started in-app notifications")
    const matchStartedEmail = screen.getByLabelText("Match Started email notifications")

    expect(matchStartedInApp).toHaveAttribute("data-state", "checked")
    expect(matchStartedEmail).toHaveAttribute("data-state", "unchecked")
  })

  it("defaults to enabled when no preference row exists (opt-out model)", async () => {
    setup([]) // empty prefs — all default to enabled
    await screen.findByText("Match Started")

    const matchStartedInApp = screen.getByLabelText("Match Started in-app notifications")
    expect(matchStartedInApp).toHaveAttribute("data-state", "checked")
  })

  it("calls updatePreference API with correct params when toggled", async () => {
    const user = userEvent.setup()
    // Make refetch return the updated (flipped) state so the component stays consistent
    vi.mocked(notificationsApi.getPreferences).mockResolvedValue({
      data: {
        preferences: MOCK_PREFERENCES.map((p) =>
          p.event_type === "match_started" && p.channel === "in_app"
            ? { ...p, enabled: false }
            : p,
        ),
      },
    } as never)

    setup()
    await screen.findByText("Match Started")

    const inAppSwitch = screen.getByLabelText("Match Started in-app notifications")
    expect(inAppSwitch).toHaveAttribute("data-state", "checked")

    await user.click(inAppSwitch)

    expect(vi.mocked(notificationsApi.updatePreference)).toHaveBeenCalledWith(
      "test-org",
      "match_started",
      { channel: "in_app", enabled: false },
    )
  })
})
