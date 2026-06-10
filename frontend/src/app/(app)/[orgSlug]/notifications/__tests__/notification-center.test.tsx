import { describe, it, expect, vi, beforeEach } from "vitest"
import { screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { renderWithProviders, makeTestQueryClient } from "@/test/test-utils"
import NotificationsPage from "../page"
import { notificationsApi } from "@/lib/api/notifications"
import { notificationKeys } from "@/lib/query-keys"
import type { Notification } from "@/types/api/notifications"
import type { InfiniteData } from "@tanstack/react-query"

vi.mock("next/navigation", () => ({
  useParams: () => ({ orgSlug: "test-org" }),
  useRouter: () => ({ push: vi.fn() }),
  usePathname: () => "/test-org/notifications",
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
    list: vi.fn().mockResolvedValue({ data: { notifications: [], total: 0, limit: 20, offset: 0 } }),
    markRead: vi.fn().mockResolvedValue({ data: { message: "ok" } }),
    markAllRead: vi.fn().mockResolvedValue({ data: { message: "ok" } }),
    delete: vi.fn().mockResolvedValue({ data: {} }),
    getPreferences: vi.fn().mockResolvedValue({ data: { preferences: [] } }),
    updatePreference: vi.fn().mockResolvedValue({ data: {} }),
  },
}))

const PAGE_LIMIT = 20

interface NotifPage {
  notifications: Notification[]
  total: number
  limit: number
  offset: number
}

function makeNotification(overrides: Partial<Notification> = {}): Notification {
  return {
    id: "n1",
    organization_id: "org1",
    user_id: "u1",
    outbox_id: "o1",
    channel: "in_app",
    event_type: "match_started",
    entity_type: "match",
    entity_id: "m1",
    payload: {},
    read_at: null,
    sent_at: null,
    created_at: new Date().toISOString(),
    ...overrides,
  }
}

// Seed both the infinite query cache and the useUnreadCount flat query cache.
function setup(notifications: Notification[] = [], total?: number) {
  const client = makeTestQueryClient()
  const resolvedTotal = total ?? notifications.length

  // Infinite query key used by NotificationsPage
  const infiniteData: InfiniteData<NotifPage, number> = {
    pages: [{ notifications, total: resolvedTotal, limit: PAGE_LIMIT, offset: 0 }],
    pageParams: [0],
  }
  client.setQueryData(notificationKeys.list("test-org", { limit: PAGE_LIMIT }), infiniteData)

  // Flat query key used by useUnreadCount
  client.setQueryData(notificationKeys.list("test-org", { limit: 50, offset: 0 }), {
    notifications,
    total: resolvedTotal,
    limit: 50,
    offset: 0,
  })

  return renderWithProviders(<NotificationsPage />, { client })
}

describe("NotificationsPage", () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it("renders loading skeleton while fetching", () => {
    renderWithProviders(<NotificationsPage />)
    const items = document.querySelectorAll(".animate-pulse")
    expect(items.length).toBeGreaterThan(0)
  })

  it("renders empty state when there are no notifications", async () => {
    setup([])
    await screen.findByText("No notifications")
    expect(screen.getByText(/real time/i)).toBeInTheDocument()
  })

  it("renders notification list items", async () => {
    setup([
      makeNotification({ id: "n1", read_at: null }),
      makeNotification({ id: "n2", read_at: "2024-01-01T00:00:00Z" }),
    ])

    await screen.findByText("2 total · 1 unread")
    const articles = screen.getAllByRole("article")
    expect(articles).toHaveLength(2)
  })

  it("shows 'Mark all read' button when there are unread notifications", async () => {
    setup([makeNotification({ id: "n1", read_at: null })])
    await screen.findByRole("button", { name: /mark all read/i })
  })

  it("hides 'Mark all read' button when all are read", async () => {
    setup([makeNotification({ id: "n1", read_at: "2024-01-01T00:00:00Z" })])
    await screen.findByText("1 total · 0 unread")
    expect(screen.queryByRole("button", { name: /mark all read/i })).toBeNull()
  })

  it("applies optimistic update when marking all as read", async () => {
    const user = userEvent.setup()
    const readAt = new Date().toISOString()
    const notifications = [
      makeNotification({ id: "n1", read_at: null }),
      makeNotification({ id: "n2", read_at: null }),
    ]

    // After onSettled invalidation refetch, return notifications with read_at set
    vi.mocked(notificationsApi.list).mockResolvedValue({
      data: {
        notifications: notifications.map((n) => ({ ...n, read_at: readAt })),
        total: 2,
        limit: PAGE_LIMIT,
        offset: 0,
      },
    } as never)

    setup(notifications)
    await screen.findByText("2 total · 2 unread")

    const markAllBtn = screen.getByRole("button", { name: /mark all read/i })
    await user.click(markAllBtn)

    await waitFor(() => {
      expect(screen.getByText(/0 unread/i)).toBeInTheDocument()
    })
  })

  it("updates list via cache invalidation (simulating SSE realtime update)", async () => {
    const { client } = setup([makeNotification({ id: "n1" })])

    await screen.findByText("1 total · 1 unread")

    // Simulate SSE-triggered cache update (new notification pushed).
    // The infinite query stores InfiniteData, so we must update in that shape.
    const updatedData: InfiniteData<NotifPage, number> = {
      pages: [
        {
          notifications: [
            makeNotification({ id: "n2", event_type: "tournament_status_changed" }),
            makeNotification({ id: "n1" }),
          ],
          total: 2,
          limit: PAGE_LIMIT,
          offset: 0,
        },
      ],
      pageParams: [0],
    }
    client.setQueryData(notificationKeys.list("test-org", { limit: PAGE_LIMIT }), updatedData)

    // Also update useUnreadCount cache
    client.setQueryData(notificationKeys.list("test-org", { limit: 50, offset: 0 }), {
      notifications: [
        makeNotification({ id: "n2", event_type: "tournament_status_changed" }),
        makeNotification({ id: "n1" }),
      ],
      total: 2,
      limit: 50,
      offset: 0,
    })

    await screen.findByText("2 total · 2 unread")
  })

  it("shows 'Load more' button when there are more pages", async () => {
    setup(
      Array.from({ length: PAGE_LIMIT }, (_, i) =>
        makeNotification({ id: `n${i}`, read_at: null }),
      ),
      PAGE_LIMIT + 5, // total > loaded → hasNextPage = true
    )

    await screen.findByRole("button", { name: /load more/i })
  })
})
