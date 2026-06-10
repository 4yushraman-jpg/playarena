import { describe, it, expect, vi, beforeEach } from "vitest"
import { screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { renderWithProviders } from "@/test/test-utils"
import SecuritySettingsPage from "../security/page"

vi.mock("next/navigation", () => ({
  useParams: () => ({ orgSlug: "test-org" }),
  useRouter: () => ({ push: vi.fn() }),
  usePathname: () => "/test-org/settings/security",
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
    changePassword: vi.fn().mockResolvedValue({ data: { message: "ok" } }),
  },
}))

import { usersApi } from "@/lib/api/users"

beforeEach(() => {
  vi.clearAllMocks()
})

describe("SecuritySettingsPage", () => {
  it("renders all three password fields", () => {
    renderWithProviders(<SecuritySettingsPage />)
    expect(screen.getByLabelText(/current password/i)).toBeInTheDocument()
    expect(screen.getByLabelText("New password")).toBeInTheDocument()
    expect(screen.getByLabelText(/confirm new password/i)).toBeInTheDocument()
  })

  it("renders the submit button", () => {
    renderWithProviders(<SecuritySettingsPage />)
    expect(screen.getByRole("button", { name: /update password/i })).toBeInTheDocument()
  })

  it("shows strength meter when new password is typed", async () => {
    const user = userEvent.setup()
    renderWithProviders(<SecuritySettingsPage />)

    const newPwField = screen.getByLabelText("New password")
    await user.type(newPwField, "abc")

    expect(screen.getByLabelText(/password requirements/i)).toBeInTheDocument()
    expect(screen.getByText(/at least 8 characters/i)).toBeInTheDocument()
  })

  it("strength meter shows 'Strong' for a fully compliant password", async () => {
    const user = userEvent.setup()
    renderWithProviders(<SecuritySettingsPage />)

    await user.type(screen.getByLabelText("New password"), "Secure123")

    expect(screen.getByText("Strong")).toBeInTheDocument()
  })

  it("calls changePassword with correct args on valid submit", async () => {
    const user = userEvent.setup()
    renderWithProviders(<SecuritySettingsPage />)

    await user.type(screen.getByLabelText(/current password/i), "OldPass1")
    await user.type(screen.getByLabelText("New password"), "NewPass1")
    await user.type(screen.getByLabelText(/confirm new password/i), "NewPass1")
    await user.click(screen.getByRole("button", { name: /update password/i }))

    await waitFor(() => {
      expect(vi.mocked(usersApi.changePassword)).toHaveBeenCalledWith("u1", {
        current_password: "OldPass1",
        new_password: "NewPass1",
      })
    })
  })

  it("shows field error when current password is incorrect", async () => {
    const user = userEvent.setup()
    vi.mocked(usersApi.changePassword).mockRejectedValueOnce({
      response: { data: { error: "Incorrect current password" } },
    })

    renderWithProviders(<SecuritySettingsPage />)

    await user.type(screen.getByLabelText(/current password/i), "WrongPass1")
    await user.type(screen.getByLabelText("New password"), "NewPass1")
    await user.type(screen.getByLabelText(/confirm new password/i), "NewPass1")
    await user.click(screen.getByRole("button", { name: /update password/i }))

    await screen.findByText(/incorrect current password/i)
  })

  it("resets form after successful password change", async () => {
    const user = userEvent.setup()
    renderWithProviders(<SecuritySettingsPage />)

    await user.type(screen.getByLabelText(/current password/i), "OldPass1")
    await user.type(screen.getByLabelText("New password"), "NewPass1")
    await user.type(screen.getByLabelText(/confirm new password/i), "NewPass1")
    await user.click(screen.getByRole("button", { name: /update password/i }))

    // Step 1: wait for mutation to fire
    await waitFor(() => {
      expect(vi.mocked(usersApi.changePassword)).toHaveBeenCalledTimes(1)
    })
    // Step 2: onSuccess calls form.reset() — wait for DOM to reflect it
    await waitFor(
      () => expect(screen.getByLabelText(/current password/i)).toHaveValue(""),
      { timeout: 3000 },
    )
  })
})
