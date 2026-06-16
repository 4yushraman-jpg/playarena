import { describe, it, expect, vi, beforeEach } from "vitest"
import { render, screen, waitFor } from "@testing-library/react"
import type { JwtClaims } from "@/types/api/auth"

// ── Controllable auth state ─────────────────────────────────────────────────
// The layout reads claims + isHydrating from the store via selectors.
let authState: { claims: JwtClaims | null; isHydrating: boolean }

vi.mock("@/stores/auth.store", () => ({
  useAuthStore: (selector?: (s: unknown) => unknown) =>
    selector ? selector(authState) : authState,
}))

// ── Navigation ──────────────────────────────────────────────────────────────
const replace = vi.fn()
let currentSlug = "tournaments"
vi.mock("next/navigation", () => ({
  useParams: () => ({ orgSlug: currentSlug }),
  useRouter: () => ({ replace, push: vi.fn() }),
  notFound: () => {
    throw new Error("notFound")
  },
}))

// ── Heavy children / side effects stubbed out ───────────────────────────────
vi.mock("@/hooks/use-notification-stream", () => ({
  useNotificationStream: vi.fn(),
}))
vi.mock("@/components/layout/org-sidebar", () => ({
  OrgSidebar: () => <nav data-testid="org-sidebar">sidebar</nav>,
}))
vi.mock("@/components/layout/org-header", () => ({
  OrgHeader: () => <header data-testid="org-header">header</header>,
}))

import OrgLayout from "../layout"
import { useNotificationStream } from "@/hooks/use-notification-stream"

function makeClaims(overrides: Partial<JwtClaims>): JwtClaims {
  return {
    userId: "u1",
    email: "user@example.com",
    organizationId: null,
    role: "onboarding",
    scope: null,
    playerProfileId: null,
    exp: 9999999999,
    ...overrides,
  }
}

describe("OrgLayout — onboarding route isolation", () => {
  beforeEach(() => {
    vi.clearAllMocks()
    currentSlug = "tournaments"
    // jsdom has no matchMedia; the layout uses it for the responsive sidebar.
    window.matchMedia = vi.fn().mockImplementation((query: string) => ({
      matches: true,
      media: query,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
    })) as unknown as typeof window.matchMedia
  })

  it("redirects an onboarding user away from a bare /tournaments path", async () => {
    authState = {
      claims: makeClaims({ role: "onboarding", scope: "onboarding" }),
      isHydrating: false,
    }
    render(<OrgLayout>org page</OrgLayout>)

    await waitFor(() => expect(replace).toHaveBeenCalledWith("/welcome"))
    // The organizer shell must not render for a user without org context.
    expect(screen.queryByTestId("org-sidebar")).not.toBeInTheDocument()
    expect(screen.queryByTestId("org-header")).not.toBeInTheDocument()
    // No notification stream should be opened for a redirecting user.
    expect(useNotificationStream).toHaveBeenCalledWith(
      expect.objectContaining({ enabled: false }),
    )
  })

  it("redirects a player user away from a bare /teams path", async () => {
    currentSlug = "teams"
    authState = {
      claims: makeClaims({ role: "viewer", scope: "player" }),
      isHydrating: false,
    }
    render(<OrgLayout>org page</OrgLayout>)

    await waitFor(() => expect(replace).toHaveBeenCalledWith("/welcome"))
    expect(screen.queryByTestId("org-sidebar")).not.toBeInTheDocument()
  })

  it("renders the organizer shell for a user with an organization", async () => {
    authState = {
      claims: makeClaims({ organizationId: "org-1", role: "org_owner", scope: "organizer" }),
      isHydrating: false,
    }
    render(<OrgLayout>org page</OrgLayout>)

    expect(screen.getByTestId("org-sidebar")).toBeInTheDocument()
    expect(screen.getByTestId("org-header")).toBeInTheDocument()
    expect(replace).not.toHaveBeenCalled()
    expect(useNotificationStream).toHaveBeenCalledWith(
      expect.objectContaining({ enabled: true }),
    )
  })

  it("renders the shell for a platform admin despite an empty organization", async () => {
    authState = {
      claims: makeClaims({ organizationId: null, role: "platform_admin", scope: "platform" }),
      isHydrating: false,
    }
    render(<OrgLayout>org page</OrgLayout>)

    expect(screen.getByTestId("org-sidebar")).toBeInTheDocument()
    expect(replace).not.toHaveBeenCalled()
  })

  it("does not redirect before auth has hydrated", async () => {
    authState = { claims: null, isHydrating: true }
    render(<OrgLayout>org page</OrgLayout>)

    expect(replace).not.toHaveBeenCalled()
  })
})
