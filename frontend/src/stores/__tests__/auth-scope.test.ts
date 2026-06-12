import { describe, it, expect, beforeEach } from "vitest"
import { useAuthStore, selectScope, selectPlayerProfileId } from "@/stores/auth.store"
import { isReservedSlug, RESERVED_SLUGS } from "@/lib/reserved-slugs"

function jwt(payload: Record<string, unknown>) {
  const header = btoa(JSON.stringify({ alg: "HS256", typ: "JWT" }))
  const body = btoa(
    JSON.stringify({
      user_id: "u1",
      email: "u@example.com",
      exp: Math.floor(Date.now() / 1000) + 3600,
      ...payload,
    }),
  )
  return `${header}.${body}.sig`
}

describe("auth store scope decoding (GP-1)", () => {
  beforeEach(() => {
    useAuthStore.getState().clearSession()
  })

  it("decodes player scope and player_profile_id", () => {
    useAuthStore.getState().setSession({
      access_token: jwt({ scope: "player", player_profile_id: "p-123", organization_id: "" }),
      refresh_token: "r",
      expires_in: 900,
      token_type: "Bearer",
    })
    const s = useAuthStore.getState()
    expect(selectScope(s)).toBe("player")
    expect(selectPlayerProfileId(s)).toBe("p-123")
    expect(s.claims?.organizationId).toBeNull()
  })

  it("decodes organizer scope with org id", () => {
    useAuthStore.getState().setSession({
      access_token: jwt({ scope: "organizer", organization_id: "org-1", role: "org_owner" }),
      refresh_token: "r",
      expires_in: 900,
      token_type: "Bearer",
    })
    const s = useAuthStore.getState()
    expect(selectScope(s)).toBe("organizer")
    expect(s.claims?.organizationId).toBe("org-1")
  })

  it("legacy token without scope decodes scope as null", () => {
    useAuthStore.getState().setSession({
      access_token: jwt({ organization_id: "org-1", role: "org_admin" }),
      refresh_token: "r",
      expires_in: 900,
      token_type: "Bearer",
    })
    expect(selectScope(useAuthStore.getState())).toBeNull()
  })
})

describe("reserved slug protection (GP-1)", () => {
  it("flags reserved segments case-insensitively", () => {
    expect(isReservedSlug("me")).toBe(true)
    expect(isReservedSlug("ME")).toBe(true)
    expect(isReservedSlug("players")).toBe(true)
    expect(isReservedSlug("onboarding")).toBe(true)
  })
  it("does not flag a normal org slug", () => {
    expect(isReservedSlug("mumbai-raiders")).toBe(false)
  })
  it("includes the routes that would collide with /me and /players", () => {
    expect(RESERVED_SLUGS.has("me")).toBe(true)
    expect(RESERVED_SLUGS.has("players")).toBe(true)
  })
})
