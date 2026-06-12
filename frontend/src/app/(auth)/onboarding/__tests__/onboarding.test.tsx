import React from "react"
import { render, screen, fireEvent, waitFor } from "@testing-library/react"
import { describe, it, expect, vi, beforeEach } from "vitest"
import { useAuthStore } from "@/stores/auth.store"

const mockPush = vi.fn()

vi.mock("next/navigation", () => ({
  useRouter: () => ({ push: mockPush }),
}))

vi.mock("@/lib/api/organizations", () => ({
  orgsApi: { create: vi.fn() },
}))

vi.mock("@/lib/api/auth", () => ({
  authApi: { refresh: vi.fn() },
}))

vi.mock("@/lib/api/client", () => ({
  tokenManager: {
    getAccessToken: vi.fn(),
    setAccessToken: vi.fn(),
    getRefreshToken: vi.fn(),
    setRefreshToken: vi.fn(),
    clearAll: vi.fn(),
  },
}))

import { authApi } from "@/lib/api/auth"
import { orgsApi } from "@/lib/api/organizations"
import { tokenManager } from "@/lib/api/client"
import OnboardingPage from "../page"

function makeMockJwt() {
  const header = btoa(JSON.stringify({ alg: "HS256", typ: "JWT" }))
  const payload = btoa(
    JSON.stringify({
      user_id: "user-1",
      email: "test@example.com",
      organization_id: "org-1",
      role: "org_owner",
      exp: Math.floor(Date.now() / 1000) + 3600,
    }),
  )
  return `${header}.${payload}.sig`
}

beforeEach(() => {
  vi.clearAllMocks()
  useAuthStore.setState({
    claims: {
      userId: "user-1",
      email: "test@example.com",
      organizationId: null,
      role: "onboarding",
      scope: "onboarding",
      playerProfileId: null,
      exp: Math.floor(Date.now() / 1000) + 3600,
    },
    orgSlug: null,
    pendingOrgSelection: null,
    isHydrating: false,
  })
  vi.mocked(tokenManager.getRefreshToken).mockReturnValue("rt-onboarding")
})

describe("OnboardingPage", () => {
  it("creates the first org, refreshes into that org, and opens the dashboard", async () => {
    vi.mocked(orgsApi.create).mockResolvedValue({
      data: {
        id: "org-1",
        name: "Alpha Club",
        slug: "alpha-club",
        type: "club",
        status: "active",
        description: null,
        website: null,
        email: null,
        phone: null,
        country: null,
        city: null,
        created_at: "2026-06-11T00:00:00Z",
        updated_at: "2026-06-11T00:00:00Z",
      },
    } as never)
    vi.mocked(authApi.refresh).mockResolvedValue({
      data: {
        access_token: makeMockJwt(),
        refresh_token: "rt-org",
        expires_in: 900,
        token_type: "Bearer",
      },
    } as never)

    render(<OnboardingPage />)

    fireEvent.change(screen.getByLabelText(/organization name/i), {
      target: { value: "Alpha Club" },
    })
    fireEvent.click(screen.getByRole("button", { name: /continue/i }))

    await waitFor(() => expect(mockPush).toHaveBeenCalledWith("/alpha-club"))
    expect(orgsApi.create).toHaveBeenCalledWith({
      name: "Alpha Club",
      type: "club",
    })
    expect(authApi.refresh).toHaveBeenCalledWith("rt-onboarding", "org-1")
    expect(useAuthStore.getState().orgSlug).toBe("alpha-club")
  })
})
