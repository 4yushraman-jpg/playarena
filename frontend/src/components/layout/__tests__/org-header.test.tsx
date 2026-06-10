import React from "react"
import { screen, fireEvent, waitFor } from "@testing-library/react"
import { describe, it, expect, vi, beforeEach } from "vitest"
import { useAuthStore } from "@/stores/auth.store"
import { renderWithProviders, makeTestQueryClient } from "@/test/test-utils"

const mockPush = vi.fn()

vi.mock("next/navigation", () => ({
  useRouter: () => ({ push: mockPush }),
}))

vi.mock("next-themes", () => ({
  useTheme: () => ({ resolvedTheme: "light", setTheme: vi.fn() }),
}))

vi.mock("@/lib/api/auth", () => ({
  authApi: { logout: vi.fn() },
}))

vi.mock("@/lib/api/client", () => ({
  tokenManager: {
    getRefreshToken: vi.fn().mockReturnValue("rt-abc"),
    getAccessToken: vi.fn().mockReturnValue(null),
    clearAll: vi.fn(),
  },
}))

// getQueryClient is a singleton; we need to control which client it returns.
vi.mock("@/lib/api/query-client", () => ({
  getQueryClient: vi.fn(),
  makeQueryClient: vi.fn(),
}))

import { authApi } from "@/lib/api/auth"
import { getQueryClient } from "@/lib/api/query-client"
import { OrgHeader } from "../org-header"

beforeEach(() => {
  vi.clearAllMocks()
  vi.mocked(authApi.logout).mockResolvedValue({ data: { message: "ok" } } as never)
  useAuthStore.setState({
    claims: {
      userId: "user-1",
      email: "user@example.com",
      organizationId: "org-1",
      role: "org_admin",
      exp: Math.floor(Date.now() / 1000) + 3600,
    },
    orgSlug: "test-org",
    pendingOrgSelection: null,
    isHydrating: false,
  })
})

describe("OrgHeader logout — P1-4 query cache cleared", () => {
  it("clears the query cache before clearing auth state on logout", async () => {
    const client = makeTestQueryClient()
    const mockClear = vi.fn()
    client.clear = mockClear
    vi.mocked(getQueryClient).mockReturnValue(client)

    renderWithProviders(<OrgHeader orgSlug="test-org" />, { client })

    const logoutBtn = screen.getByRole("button", { name: /sign out/i })
    fireEvent.click(logoutBtn)

    await waitFor(() => expect(mockClear).toHaveBeenCalledTimes(1))
  })

  it("clears auth state after clearing the query cache", async () => {
    const client = makeTestQueryClient()
    const callOrder: string[] = []
    client.clear = vi.fn(() => { callOrder.push("cache_cleared") })
    vi.mocked(getQueryClient).mockReturnValue(client)

    const clearSessionSpy = vi.spyOn(useAuthStore.getState(), "clearSession").mockImplementation(() => {
      callOrder.push("session_cleared")
    })

    renderWithProviders(<OrgHeader orgSlug="test-org" />, { client })

    fireEvent.click(screen.getByRole("button", { name: /sign out/i }))

    await waitFor(() => expect(callOrder).toContain("session_cleared"))

    expect(callOrder.indexOf("cache_cleared")).toBeLessThan(callOrder.indexOf("session_cleared"))

    clearSessionSpy.mockRestore()
  })

  it("redirects to /login after logout", async () => {
    const client = makeTestQueryClient()
    vi.mocked(getQueryClient).mockReturnValue(client)

    renderWithProviders(<OrgHeader orgSlug="test-org" />, { client })

    fireEvent.click(screen.getByRole("button", { name: /sign out/i }))

    await waitFor(() => expect(mockPush).toHaveBeenCalledWith("/login"))
  })
})
