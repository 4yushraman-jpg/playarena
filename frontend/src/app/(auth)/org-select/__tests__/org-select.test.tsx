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

vi.mock("sonner", () => ({
  toast: { error: vi.fn() },
}))

import { authApi } from "@/lib/api/auth"
import OrgSelectPage from "../page"

function makeMockJwt() {
  const header = btoa(JSON.stringify({ alg: "HS256", typ: "JWT" }))
  const payload = btoa(
    JSON.stringify({
      user_id: "user-1",
      email: "test@example.com",
      organization_id: "org-1",
      role: "org_admin",
      exp: Math.floor(Date.now() / 1000) + 3600,
    }),
  )
  return `${header}.${payload}.sig`
}

beforeEach(() => {
  vi.clearAllMocks()
  // Set up sessionStorage with temporary credentials
  sessionStorage.setItem("pa_pending_email", "user@example.com")
  sessionStorage.setItem("pa_pending_password", "password123")
  // Pre-populate store with pending org list
  useAuthStore.setState({
    claims: null,
    orgSlug: null,
    pendingOrgSelection: [
      { id: "org-1", name: "Alpha FC", slug: "alpha-fc" },
      { id: "org-2", name: "Beta Club", slug: "beta-club" },
    ],
    isHydrating: false,
  })
})

describe("OrgSelectPage — P0-2 race condition fix", () => {
  it("renders org list when pendingOrgSelection is populated", () => {
    render(<OrgSelectPage />)
    expect(screen.getByText("Alpha FC")).toBeInTheDocument()
    expect(screen.getByText("Beta Club")).toBeInTheDocument()
  })

  it("redirects to /login (not /org) when pendingOrgSelection is empty and orgSlug is null", () => {
    useAuthStore.setState({ pendingOrgSelection: null, orgSlug: null })
    render(<OrgSelectPage />)
    expect(mockReplace).toHaveBeenCalledWith("/login")
    expect(mockPush).not.toHaveBeenCalled()
  })

  it("successful org selection pushes to /{orgSlug} and does NOT redirect to /login", async () => {
    vi.mocked(authApi.login).mockResolvedValue({
      data: {
        access_token: makeMockJwt(),
        refresh_token: "rt-new",
        expires_in: 900,
        token_type: "Bearer",
      },
    } as never)

    render(<OrgSelectPage />)

    fireEvent.click(screen.getByText("Alpha FC"))

    await waitFor(() => expect(mockPush).toHaveBeenCalledWith("/alpha-fc"))

    // The race: ensure router.replace("/login") was NOT called after successful selection
    expect(mockReplace).not.toHaveBeenCalledWith("/login")
  })

  it("clears sessionStorage credentials after successful selection", async () => {
    vi.mocked(authApi.login).mockResolvedValue({
      data: {
        access_token: makeMockJwt(),
        refresh_token: "rt-new",
        expires_in: 900,
        token_type: "Bearer",
      },
    } as never)

    render(<OrgSelectPage />)
    fireEvent.click(screen.getByText("Beta Club"))

    await waitFor(() => expect(mockPush).toHaveBeenCalledWith("/beta-club"))

    expect(sessionStorage.getItem("pa_pending_email")).toBeNull()
    expect(sessionStorage.getItem("pa_pending_password")).toBeNull()
  })

  it("does not redirect to /login when pendingOrgSelection clears but orgSlug is now set", () => {
    // Simulate the state after a successful org selection lands:
    // pendingOrgSelection = null, orgSlug = "alpha-fc"
    useAuthStore.setState({ pendingOrgSelection: null, orgSlug: "alpha-fc" })
    render(<OrgSelectPage />)
    // Guard should NOT redirect because orgSlug is set
    expect(mockReplace).not.toHaveBeenCalledWith("/login")
  })
})
