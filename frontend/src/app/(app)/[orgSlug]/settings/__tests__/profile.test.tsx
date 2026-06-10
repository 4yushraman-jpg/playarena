import { describe, it, expect, vi, beforeEach } from "vitest"
import { screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { renderWithProviders, makeTestQueryClient } from "@/test/test-utils"
import ProfileSettingsPage from "../profile/page"
import { userKeys } from "@/lib/query-keys"
import type { User } from "@/types/api/users"

const MOCK_USER: User = {
  id: "u1",
  email: "test@example.com",
  username: "testuser",
  full_name: "Test User",
  status: "active",
  email_verified_at: null,
  last_login_at: null,
  last_login_ip: null,
  created_at: "2024-01-01T00:00:00Z",
  updated_at: "2024-01-01T00:00:00Z",
}

vi.mock("next/navigation", () => ({
  useParams: () => ({ orgSlug: "test-org" }),
  useRouter: () => ({ push: vi.fn() }),
  usePathname: () => "/test-org/settings/profile",
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

vi.mock("@/lib/api/users", () => ({
  usersApi: {
    get: vi.fn().mockResolvedValue({
      data: {
        id: "u1",
        email: "test@example.com",
        username: "testuser",
        full_name: "Test User",
        status: "active",
        email_verified_at: null,
        last_login_at: null,
        last_login_ip: null,
        created_at: "2024-01-01T00:00:00Z",
        updated_at: "2024-01-01T00:00:00Z",
      },
    }),
    update: vi.fn().mockResolvedValue({
      data: {
        id: "u1",
        email: "test@example.com",
        username: "testuser",
        full_name: "Test User",
        status: "active",
        email_verified_at: null,
        last_login_at: null,
        last_login_ip: null,
        created_at: "2024-01-01T00:00:00Z",
        updated_at: "2024-01-01T00:00:00Z",
      },
    }),
    changePassword: vi.fn().mockResolvedValue({ data: { message: "ok" } }),
  },
}))

// Pre-seed the cache so queries resolve from cache without network
function setup(overrides?: Partial<User>) {
  const client = makeTestQueryClient()
  client.setQueryData(userKeys.detail("u1"), { ...MOCK_USER, ...overrides })
  return renderWithProviders(<ProfileSettingsPage />, { client })
}

describe("ProfileSettingsPage", () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it("shows loading skeleton while user data is fetching", () => {
    renderWithProviders(<ProfileSettingsPage />)
    const skeletons = document.querySelectorAll(".animate-pulse")
    expect(skeletons.length).toBeGreaterThan(0)
  })

  it("renders form fields after user data loads", async () => {
    setup()
    await screen.findByDisplayValue("Test User")
    await screen.findByDisplayValue("testuser")
    // email appears twice: avatar section + read-only field
    expect(screen.getAllByText("test@example.com").length).toBeGreaterThanOrEqual(1)
  })

  it("save button is disabled when form is clean", async () => {
    setup()
    await screen.findByDisplayValue("Test User")
    const saveButton = screen.getByRole("button", { name: /save changes/i })
    expect(saveButton).toBeDisabled()
  })

  it("save button enables when form is dirty", async () => {
    const user = userEvent.setup()
    setup()

    const fullNameInput = await screen.findByDisplayValue("Test User")
    await user.clear(fullNameInput)
    await user.type(fullNameInput, "Updated Name")

    const saveButton = screen.getByRole("button", { name: /save changes/i })
    expect(saveButton).not.toBeDisabled()
  })

  it("shows unsaved changes indicator when dirty", async () => {
    const user = userEvent.setup()
    setup()

    const fullNameInput = await screen.findByDisplayValue("Test User")
    await user.clear(fullNameInput)
    await user.type(fullNameInput, "Changed Name")

    expect(screen.getByText(/unsaved changes/i)).toBeInTheDocument()
  })

  it("shows cancel button when form is dirty", async () => {
    const user = userEvent.setup()
    setup()

    const fullNameInput = await screen.findByDisplayValue("Test User")
    await user.clear(fullNameInput)
    await user.type(fullNameInput, "Changed Name")

    expect(screen.getByRole("button", { name: /cancel/i })).toBeInTheDocument()
  })

  it("resets form on cancel", async () => {
    const user = userEvent.setup()
    setup()

    const fullNameInput = await screen.findByDisplayValue("Test User")
    await user.clear(fullNameInput)
    await user.type(fullNameInput, "Changed Name")

    const cancelButton = screen.getByRole("button", { name: /cancel/i })
    await user.click(cancelButton)

    await screen.findByDisplayValue("Test User")
  })

  it("shows inline validation for short username", async () => {
    const user = userEvent.setup()
    setup()

    const usernameInput = await screen.findByDisplayValue("testuser")
    await user.clear(usernameInput)
    await user.type(usernameInput, "ab")
    await user.tab()

    await screen.findByText(/at least 3 characters/i)
  })
})
