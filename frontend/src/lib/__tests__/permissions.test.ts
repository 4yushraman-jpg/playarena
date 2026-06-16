import { describe, it, expect } from "vitest"
import { hasOrgContext } from "@/lib/permissions"
import type { JwtClaims } from "@/types/api/auth"
import type { Role } from "@/types/common"
import type { Scope } from "@/types/api/auth"

function makeClaims(overrides: Partial<JwtClaims>): JwtClaims {
  return {
    userId: "u1",
    email: "user@example.com",
    organizationId: null,
    role: "onboarding" as Role,
    scope: null as Scope | null,
    playerProfileId: null,
    exp: 9999999999,
    ...overrides,
  }
}

describe("hasOrgContext", () => {
  it("denies a null (unauthenticated) claims object", () => {
    expect(hasOrgContext(null)).toBe(false)
  })

  it("denies an onboarding user with no org context", () => {
    expect(hasOrgContext(makeClaims({ role: "onboarding", scope: "onboarding" }))).toBe(false)
  })

  it("denies a player user with no org context", () => {
    expect(hasOrgContext(makeClaims({ role: "viewer", scope: "player" }))).toBe(false)
  })

  it("allows an organizer user who carries an organizationId", () => {
    expect(
      hasOrgContext(makeClaims({ organizationId: "org-1", role: "org_owner", scope: "organizer" })),
    ).toBe(true)
  })

  it("allows a platform user via the platform scope despite an empty org", () => {
    expect(
      hasOrgContext(makeClaims({ organizationId: null, role: "platform_admin", scope: "platform" })),
    ).toBe(true)
  })

  it("allows a legacy platform token by role when the scope claim is absent", () => {
    expect(
      hasOrgContext(makeClaims({ organizationId: null, role: "platform_admin", scope: null })),
    ).toBe(true)
  })

  it("allows an organizer token minted before the scope claim existed", () => {
    expect(hasOrgContext(makeClaims({ organizationId: "org-1", role: "org_admin", scope: null }))).toBe(
      true,
    )
  })
})
