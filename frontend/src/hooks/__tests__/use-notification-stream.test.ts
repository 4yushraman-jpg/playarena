import { act, waitFor } from "@testing-library/react"
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest"
import { useAuthStore } from "@/stores/auth.store"
import { renderHookWithProviders } from "@/test/test-utils"

// ── Mock EventSource ──────────────────────────────────────────────────────────

type EventHandler = (ev: Event) => void

class MockEventSource {
  static instances: MockEventSource[] = []
  url: string
  onopen: EventHandler | null = null
  onerror: EventHandler | null = null
  onmessage: ((ev: MessageEvent) => void) | null = null
  private listeners = new Map<string, EventHandler[]>()
  readyState = 1

  constructor(url: string) {
    this.url = url
    MockEventSource.instances.push(this)
  }

  addEventListener(type: string, fn: EventHandler) {
    if (!this.listeners.has(type)) this.listeners.set(type, [])
    this.listeners.get(type)!.push(fn)
  }

  emit(type: string, event: Partial<Event> = {}) {
    this.listeners.get(type)?.forEach((fn) => fn(event as Event))
  }

  close() { this.readyState = 2 }

  static OPEN = 1
  static CLOSED = 2
  static CONNECTING = 0
}

// ── Module mocks ──────────────────────────────────────────────────────────────

const mockReplace = vi.fn()

vi.mock("next/navigation", () => ({
  useRouter: () => ({ replace: mockReplace }),
}))

vi.mock("@/lib/api/client", () => ({
  tokenManager: {
    getAccessToken: vi.fn(),
    getRefreshToken: vi.fn(),
    clearAll: vi.fn(),
  },
  attemptTokenRefresh: vi.fn(),
}))

vi.mock("@/lib/api/query-client", () => ({
  getQueryClient: vi.fn().mockReturnValue({ clear: vi.fn() }),
  makeQueryClient: vi.fn(),
}))

import { tokenManager, attemptTokenRefresh } from "@/lib/api/client"
import { getQueryClient } from "@/lib/api/query-client"
import { useNotificationStream } from "../use-notification-stream"
import { makeTestQueryClient } from "@/test/test-utils"
import { notificationKeys } from "@/lib/query-keys"

beforeEach(() => {
  vi.clearAllMocks()
  MockEventSource.instances = []
  vi.stubGlobal("EventSource", MockEventSource)

  vi.mocked(tokenManager.getAccessToken).mockReturnValue("access-token-1")
  vi.mocked(tokenManager.getRefreshToken).mockReturnValue("refresh-token")
  vi.mocked(attemptTokenRefresh).mockResolvedValue("access-token-2")

  useAuthStore.setState({
    claims: null,
    orgSlug: "my-org",
    pendingOrgSelection: null,
    isHydrating: false,
  })
})

afterEach(() => {
  vi.unstubAllGlobals()
})

describe("useNotificationStream — SSE invalidation", () => {
  it("invalidates notificationKeys.all(orgSlug) when a message is received", async () => {
    const client = makeTestQueryClient()
    const invalidateSpy = vi.spyOn(client, "invalidateQueries")

    renderHookWithProviders(() => useNotificationStream({ orgSlug: "my-org" }), { client })

    const es = MockEventSource.instances[0]
    act(() => {
      if (es.onmessage) {
        es.onmessage({
          data: JSON.stringify({
            id: "n1",
            event_type: "match_started",
            entity_type: "match",
            entity_id: "m1",
            payload: {},
            created_at: new Date().toISOString(),
          }),
        } as MessageEvent)
      }
    })

    await waitFor(() => {
      expect(invalidateSpy).toHaveBeenCalledWith(
        expect.objectContaining({ queryKey: notificationKeys.all("my-org") }),
      )
    })
  })
})

describe("useNotificationStream — P0-3 auth_error fix", () => {
  it("creates an EventSource with the access token in the URL on mount", () => {
    renderHookWithProviders(() => useNotificationStream({ orgSlug: "my-org" }))

    expect(MockEventSource.instances).toHaveLength(1)
    expect(MockEventSource.instances[0].url).toContain("token=access-token-1")
  })

  it("calls attemptTokenRefresh when auth_error event fires", async () => {
    renderHookWithProviders(() => useNotificationStream({ orgSlug: "my-org" }))

    const es = MockEventSource.instances[0]

    act(() => {
      es.emit("auth_error")
    })

    await waitFor(() => expect(attemptTokenRefresh).toHaveBeenCalledTimes(1))
  })

  it("reconnects with a NEW EventSource after successful token refresh", async () => {
    vi.useFakeTimers()

    renderHookWithProviders(() => useNotificationStream({ orgSlug: "my-org" }))

    expect(MockEventSource.instances).toHaveLength(1)
    vi.mocked(tokenManager.getAccessToken).mockReturnValue("access-token-2")

    // Emit auth_error, then drain all timers + pending promises together.
    // vi.runAllTimersAsync() advances fake timers AND awaits microtasks so
    // the promise chain (emit → refresh → setTimeout → connect) fully resolves.
    act(() => { MockEventSource.instances[0].emit("auth_error") })
    await act(() => vi.runAllTimersAsync())

    expect(attemptTokenRefresh).toHaveBeenCalledTimes(1)
    expect(MockEventSource.instances).toHaveLength(2)
    expect(MockEventSource.instances[1].url).toContain("token=access-token-2")

    vi.useRealTimers()
  })

  it("redirects to /login and does NOT reconnect when token refresh fails", async () => {
    vi.mocked(attemptTokenRefresh).mockResolvedValue(null)
    vi.useFakeTimers()

    renderHookWithProviders(() => useNotificationStream({ orgSlug: "my-org" }))

    act(() => { MockEventSource.instances[0].emit("auth_error") })
    await act(() => vi.runAllTimersAsync())

    // Refresh returned null — no reconnect, redirect to /login
    expect(MockEventSource.instances).toHaveLength(1)
    expect(mockReplace).toHaveBeenCalledWith("/login")

    vi.useRealTimers()
  })

  it("clears the query cache when token refresh fails on auth_error", async () => {
    vi.mocked(attemptTokenRefresh).mockResolvedValue(null)
    vi.useFakeTimers()

    const client = makeTestQueryClient()
    const clearSpy = vi.spyOn(client, "clear")
    // Route getQueryClient() to the test client so the spy fires correctly
    vi.mocked(getQueryClient).mockReturnValueOnce(client as never)

    renderHookWithProviders(() => useNotificationStream({ orgSlug: "my-org" }), { client })

    act(() => { MockEventSource.instances[0].emit("auth_error") })
    await act(() => vi.runAllTimersAsync())

    expect(clearSpy).toHaveBeenCalledTimes(1)

    vi.useRealTimers()
  })

  it("does not create an infinite loop on repeated auth_error events", async () => {
    vi.useFakeTimers()

    renderHookWithProviders(() => useNotificationStream({ orgSlug: "my-org" }))

    // First auth_error — refresh succeeds, reconnects
    act(() => { MockEventSource.instances[0].emit("auth_error") })
    await act(() => vi.runAllTimersAsync())
    expect(MockEventSource.instances).toHaveLength(2)

    // Second auth_error on reconnected stream
    act(() => { MockEventSource.instances[1].emit("auth_error") })
    await act(() => vi.runAllTimersAsync())
    expect(MockEventSource.instances).toHaveLength(3)

    // Each auth_error causes exactly one refresh call — no runaway loop
    expect(attemptTokenRefresh).toHaveBeenCalledTimes(2)

    vi.useRealTimers()
  })
})
