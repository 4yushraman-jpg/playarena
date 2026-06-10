import React from "react"
import { screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { describe, it, expect, vi, beforeEach } from "vitest"
import { useAuthStore } from "@/stores/auth.store"
import { renderWithProviders, makeTestQueryClient } from "@/test/test-utils"
import { orgKeys } from "@/lib/query-keys"

const mockPush = vi.fn()

vi.mock("next/navigation", () => ({
  useRouter: () => ({ push: mockPush }),
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

vi.mock("@/lib/api/query-client", () => ({
  getQueryClient: vi.fn(),
  makeQueryClient: vi.fn(),
}))

vi.mock("@/lib/api/organizations", () => ({
  orgsApi: {
    list: vi.fn().mockResolvedValue({ data: { organizations: [], total: 0, limit: 20, offset: 0 } }),
  },
}))

import { authApi } from "@/lib/api/auth"
import { getQueryClient } from "@/lib/api/query-client"
import { OrgSwitcher } from "../org-switcher"

const ORG_A = { id: "org-a-id", name: "Org Alpha", slug: "org-a", created_at: "2024-01-01T00:00:00Z", updated_at: "2024-01-01T00:00:00Z" }
const ORG_B = { id: "org-b-id", name: "Org Beta", slug: "org-b", created_at: "2024-01-01T00:00:00Z", updated_at: "2024-01-01T00:00:00Z" }

beforeEach(() => {
  vi.clearAllMocks()
  vi.mocked(authApi.logout).mockResolvedValue({ data: { message: "ok" } } as never)
  useAuthStore.setState({
    claims: {
      userId: "user-1",
      email: "user@example.com",
      organizationId: "org-a-id",
      role: "org_owner",
      exp: Math.floor(Date.now() / 1000) + 3600,
    },
    orgSlug: "org-a",
    pendingOrgSelection: null,
    isHydrating: false,
  })
})

function setupClient(orgs: typeof ORG_A[]) {
  const client = makeTestQueryClient()
  const mockClear = vi.fn()
  client.clear = mockClear
  vi.mocked(getQueryClient).mockReturnValue(client as never)
  // OrgSwitcher queries orgKeys.list() (no params) — seed that exact key
  client.setQueryData(orgKeys.list(), {
    organizations: orgs,
    total: orgs.length,
    limit: 20,
    offset: 0,
  })
  return { client, mockClear }
}

describe("OrgSwitcher", () => {
  it("renders org name as static text when only one org exists", async () => {
    const { client } = setupClient([ORG_A])
    renderWithProviders(<OrgSwitcher currentOrgSlug="org-a" />, { client })

    await screen.findByText("Org Alpha")
    expect(screen.queryByRole("button", { name: /current organization/i })).toBeNull()
  })

  it("renders a dropdown trigger when multiple orgs exist", async () => {
    const { client } = setupClient([ORG_A, ORG_B])
    renderWithProviders(<OrgSwitcher currentOrgSlug="org-a" />, { client })

    await screen.findByRole("button", { name: /current organization/i })
  })

  it("lists other orgs in the dropdown", async () => {
    const user = userEvent.setup()
    const { client } = setupClient([ORG_A, ORG_B])
    renderWithProviders(<OrgSwitcher currentOrgSlug="org-a" />, { client })

    await user.click(await screen.findByRole("button", { name: /current organization/i }))

    await screen.findByText("Org Beta")
    expect(screen.getByText("Switch organization")).toBeInTheDocument()
  })

  it("calls logout, clears query cache, and redirects to /login on org switch", async () => {
    const user = userEvent.setup()
    const { client, mockClear } = setupClient([ORG_A, ORG_B])
    renderWithProviders(<OrgSwitcher currentOrgSlug="org-a" />, { client })

    await user.click(await screen.findByRole("button", { name: /current organization/i }))
    await user.click(await screen.findByText("Org Beta"))

    await waitFor(() => {
      expect(vi.mocked(authApi.logout)).toHaveBeenCalledWith({ refresh_token: "rt-abc" })
    })
    await waitFor(() => expect(mockClear).toHaveBeenCalledTimes(1))
    await waitFor(() => {
      expect(mockPush).toHaveBeenCalledWith("/login")
    })
    // No query params — ?email and ?next were removed because the login page did not consume them
    expect(mockPush.mock.calls[0][0]).not.toContain("?")
  })
})
