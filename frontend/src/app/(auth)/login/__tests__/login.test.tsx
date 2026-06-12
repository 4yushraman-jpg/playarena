import React from "react"
import { render, screen, fireEvent, waitFor } from "@testing-library/react"
import { describe, it, expect, vi, beforeEach } from "vitest"
import { useAuthStore } from "@/stores/auth.store"

const mockPush = vi.fn()
const mockReplace = vi.fn()

vi.mock("next/navigation", () => ({
  useRouter: () => ({ push: mockPush, replace: mockReplace }),
}))

vi.mock("@/lib/api/auth", () => ({
  authApi: { login: vi.fn() },
}))

vi.mock("@/lib/api/organizations", () => ({
  orgsApi: { list: vi.fn() },
}))

import { authApi } from "@/lib/api/auth"
import { orgsApi } from "@/lib/api/organizations"
import LoginPage from "../page"

function makeMockJwt(organizationId: string, role: string) {
  const header = btoa(JSON.stringify({ alg: "HS256", typ: "JWT" }))
  const payload = btoa(
    JSON.stringify({
      user_id: "user-1",
      email: "test@example.com",
      organization_id: organizationId,
      role,
      exp: Math.floor(Date.now() / 1000) + 3600,
    }),
  )
  return `${header}.${payload}.sig`
}

function tokenResponse(accessToken: string) {
  return {
    data: {
      access_token: accessToken,
      refresh_token: "rt-1",
      expires_in: 900,
      token_type: "Bearer",
    },
  } as never
}

async function submitLogin() {
  fireEvent.change(screen.getByLabelText(/email/i), {
    target: { value: "test@example.com" },
  })
  fireEvent.change(screen.getByLabelText(/password/i), {
    target: { value: "Password1!" },
  })
  fireEvent.click(screen.getByRole("button", { name: /sign in/i }))
}

beforeEach(() => {
  vi.clearAllMocks()
  sessionStorage.clear()
  localStorage.clear()
  useAuthStore.setState({
    claims: null,
    orgSlug: null,
    pendingOrgSelection: null,
    isHydrating: false,
  })
})

describe("LoginPage — login completion", () => {
  it("single-org user: logs in, resolves the token's org slug, opens dashboard", async () => {
    vi.mocked(authApi.login).mockResolvedValue(
      tokenResponse(makeMockJwt("org-1", "org_owner")),
    )
    vi.mocked(orgsApi.list).mockResolvedValue({
      data: {
        organizations: [
          { id: "org-other", name: "Other", slug: "other" },
          { id: "org-1", name: "Alpha FC", slug: "alpha-fc" },
        ],
        total: 2,
        limit: 200,
        offset: 0,
      },
    } as never)

    render(<LoginPage />)
    await submitLogin()

    await waitFor(() => expect(mockPush).toHaveBeenCalledWith("/alpha-fc"))
    expect(useAuthStore.getState().orgSlug).toBe("alpha-fc")
    expect(useAuthStore.getState().claims?.organizationId).toBe("org-1")
  })

  it("zero-org user: onboarding token redirects to /onboarding without an org lookup", async () => {
    vi.mocked(authApi.login).mockResolvedValue(
      tokenResponse(makeMockJwt("", "onboarding")),
    )

    render(<LoginPage />)
    await submitLogin()

    await waitFor(() => expect(mockPush).toHaveBeenCalledWith("/onboarding"))
    expect(orgsApi.list).not.toHaveBeenCalled()
    expect(useAuthStore.getState().claims?.role).toBe("onboarding")
  })

  it("multi-org user: 409 stores pending orgs + credentials and redirects to /org-select", async () => {
    vi.mocked(authApi.login).mockRejectedValue({
      response: {
        status: 409,
        data: {
          error: "organization_id is required",
          code: "organization_required",
          organizations: [
            { id: "org-1", name: "Alpha FC", slug: "alpha-fc" },
            { id: "org-2", name: "Beta Club", slug: "beta-club" },
          ],
        },
      },
    })

    render(<LoginPage />)
    await submitLogin()

    await waitFor(() => expect(mockPush).toHaveBeenCalledWith("/org-select"))
    expect(useAuthStore.getState().pendingOrgSelection).toHaveLength(2)
    expect(sessionStorage.getItem("pa_pending_email")).toBe("test@example.com")
    expect(sessionStorage.getItem("pa_pending_password")).toBe("Password1!")
  })

  it("invalid credentials: shows the API error and stays on the page", async () => {
    vi.mocked(authApi.login).mockRejectedValue({
      response: { status: 401, data: { error: "invalid credentials" } },
    })

    render(<LoginPage />)
    await submitLogin()

    expect(await screen.findByRole("alert")).toHaveTextContent("invalid credentials")
    expect(mockPush).not.toHaveBeenCalled()
  })
})
