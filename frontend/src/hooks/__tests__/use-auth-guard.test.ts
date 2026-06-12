import { renderHook, waitFor } from "@testing-library/react"
import { describe, it, expect, vi, beforeEach } from "vitest"
import { useAuthStore } from "@/stores/auth.store"

// Build a structurally valid JWT with a far-future expiry.
function makeMockJwt(overrides: Record<string, unknown> = {}) {
  const header = btoa(JSON.stringify({ alg: "HS256", typ: "JWT" }))
  const payload = btoa(
    JSON.stringify({
      user_id: "user-1",
      email: "test@example.com",
      organization_id: "org-1",
      role: "org_admin",
      scope: "organizer",
      player_profile_id: null,
      exp: Math.floor(Date.now() / 1000) + 3600,
      ...overrides,
    }),
  )
  return `${header}.${payload}.sig`
}

const mockReplace = vi.fn()
const MOCK_TOKEN = makeMockJwt()

vi.mock("next/navigation", () => ({
  useRouter: () => ({ replace: mockReplace }),
}))

vi.mock("@/lib/api/auth", () => ({
  authApi: { me: vi.fn() },
}))

vi.mock("@/lib/api/organizations", () => ({
  orgsApi: { list: vi.fn() },
}))

vi.mock("@/lib/api/client", () => ({
  tokenManager: {
    getRefreshToken: vi.fn(),
    getAccessToken: vi.fn(),
    clearAll: vi.fn(),
  },
}))

import { tokenManager } from "@/lib/api/client"
import { authApi } from "@/lib/api/auth"
import { orgsApi } from "@/lib/api/organizations"
import { useAuthGuard } from "../use-auth-guard"

beforeEach(() => {
  vi.clearAllMocks()
  useAuthStore.setState({
    claims: null,
    orgSlug: null,
    pendingOrgSelection: null,
    isHydrating: true,
  })
  vi.mocked(tokenManager.getRefreshToken).mockReturnValue("rt-abc")
  vi.mocked(tokenManager.getAccessToken).mockReturnValue(MOCK_TOKEN)
  vi.mocked(authApi.me).mockResolvedValue({ data: {} } as never)
  vi.mocked(orgsApi.list).mockResolvedValue({
    data: { organizations: [{ id: "org-1", name: "Test Org", slug: "test-org" }] },
  } as never)
})

describe("useAuthGuard — P0-1 claims hydration", () => {
  it("populates claims after successful me() call", async () => {
    const { result } = renderHook(() => useAuthGuard())

    await waitFor(() => expect(result.current.isHydrating).toBe(false))

    const { claims } = useAuthStore.getState()
    expect(claims).not.toBeNull()
    expect(claims?.email).toBe("test@example.com")
    expect(claims?.userId).toBe("user-1")
    expect(claims?.role).toBe("org_admin")
  })

  it("resolves orgSlug from /organizations after successful me()", async () => {
    const { result } = renderHook(() => useAuthGuard())

    await waitFor(() => expect(result.current.isHydrating).toBe(false))

    expect(useAuthStore.getState().orgSlug).toBe("test-org")
    expect(result.current.orgSlug).toBe("test-org")
  })

  it("redirects to /login when no refresh token is present", async () => {
    vi.mocked(tokenManager.getRefreshToken).mockReturnValue(null)

    renderHook(() => useAuthGuard())

    await waitFor(() => expect(mockReplace).toHaveBeenCalledWith("/login"))
    expect(useAuthStore.getState().claims).toBeNull()
  })

  it("redirects to /login when me() rejects (refresh failed)", async () => {
    vi.mocked(authApi.me).mockRejectedValue(new Error("401"))

    renderHook(() => useAuthGuard())

    await waitFor(() => expect(mockReplace).toHaveBeenCalledWith("/login"))
  })

  it("skips me() call and does not overwrite existing valid claims", async () => {
    // Pre-populate the store as if login already ran this session.
    useAuthStore.setState({
      claims: {
        userId: "user-1",
        email: "test@example.com",
        organizationId: "org-1",
        role: "org_admin",
        scope: "organizer",
        playerProfileId: null,
        exp: Math.floor(Date.now() / 1000) + 3600,
      },
      orgSlug: "test-org",
      isHydrating: true,
    })

    const { result } = renderHook(() => useAuthGuard())

    await waitFor(() => expect(result.current.isHydrating).toBe(false))

    // me() should not have been called because the token is still valid
    expect(vi.mocked(authApi.me)).not.toHaveBeenCalled()
    expect(useAuthStore.getState().claims?.email).toBe("test@example.com")
  })

  it("keeps onboarding sessions unscoped and does not resolve an org slug", async () => {
    vi.mocked(tokenManager.getAccessToken).mockReturnValue(makeMockJwt({
      organization_id: "",
      role: "onboarding",
    }))

    const { result } = renderHook(() => useAuthGuard())

    await waitFor(() => expect(result.current.isHydrating).toBe(false))

    expect(useAuthStore.getState().claims?.role).toBe("onboarding")
    expect(useAuthStore.getState().claims?.organizationId).toBeNull()
    expect(useAuthStore.getState().orgSlug).toBeNull()
    expect(vi.mocked(orgsApi.list)).not.toHaveBeenCalled()
  })
})
