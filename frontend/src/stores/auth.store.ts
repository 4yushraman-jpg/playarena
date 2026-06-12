import { create } from "zustand"
import { tokenManager } from "@/lib/api/client"
import type { JwtClaims, OrgSummary, Scope, TokenResponse } from "@/types/api/auth"
import type { Role } from "@/types/common"

function decodeJwt(token: string): JwtClaims | null {
  try {
    const payload = token.split(".")[1]
    if (!payload) return null
    const decoded = JSON.parse(atob(payload.replace(/-/g, "+").replace(/_/g, "/")))
    return {
      userId: decoded.user_id,
      email: decoded.email,
      organizationId: decoded.organization_id || null,
      role: decoded.role as Role,
      scope: (decoded.scope as Scope) || null,
      playerProfileId: decoded.player_profile_id || null,
      exp: decoded.exp,
    }
  } catch {
    return null
  }
}

interface AuthState {
  // Decoded claims from the current access token.
  // null means unauthenticated.
  claims: JwtClaims | null

  // The org slug (URL segment) for the authenticated org.
  // Populated after login by fetching /organizations.
  orgSlug: string | null

  // List of orgs returned by a 409 during login.
  // Cleared once the user picks an org and logs in.
  pendingOrgSelection: OrgSummary[] | null

  // Whether a silent refresh is in progress (suppresses flickers on load).
  isHydrating: boolean

  // ── Actions ──────────────────────────────────────────────────────────────────

  /** Called after a successful login or token refresh. */
  setSession: (tokens: TokenResponse) => void

  /**
   * Decode the current access token from storage and write claims to the store.
   * Used after a silent refresh (401 interceptor) where the new token has been
   * written to sessionStorage but the store has not been updated.
   */
  hydrateClaims: () => void

  /** Called after determining the org slug (requires separate API call). */
  setOrgSlug: (slug: string) => void

  /** Clear everything — called on logout or after a failed refresh. */
  clearSession: () => void

  /** Set pending org list from a 409 response. */
  setPendingOrgSelection: (orgs: OrgSummary[]) => void

  clearPendingOrgSelection: () => void

  setHydrating: (v: boolean) => void
}

export const useAuthStore = create<AuthState>((set) => ({
  claims: null,
  orgSlug: null,
  pendingOrgSelection: null,
  isHydrating: true,

  setSession: (tokens) => {
    tokenManager.setAccessToken(tokens.access_token)
    tokenManager.setRefreshToken(tokens.refresh_token)
    const claims = decodeJwt(tokens.access_token)
    set({ claims, pendingOrgSelection: null, isHydrating: false })
  },

  hydrateClaims: () => {
    const token = tokenManager.getAccessToken()
    if (!token) return
    const decoded = decodeJwt(token)
    if (decoded) set({ claims: decoded })
  },

  setOrgSlug: (slug) => set({ orgSlug: slug }),

  clearSession: () => {
    tokenManager.clearAll()
    set({ claims: null, orgSlug: null, pendingOrgSelection: null, isHydrating: false })
  },

  setPendingOrgSelection: (orgs) => {
    set({ pendingOrgSelection: orgs })
  },

  clearPendingOrgSelection: () => {
    set({ pendingOrgSelection: null })
  },

  setHydrating: (v) => set({ isHydrating: v }),
}))

// ── Selectors ─────────────────────────────────────────────────────────────────

export const selectIsAuthenticated = (s: AuthState) => s.claims !== null
export const selectOrgId = (s: AuthState) => s.claims?.organizationId ?? null
export const selectRole = (s: AuthState) => s.claims?.role ?? null
export const selectUserId = (s: AuthState) => s.claims?.userId ?? null
export const selectScope = (s: AuthState) => s.claims?.scope ?? null
export const selectPlayerProfileId = (s: AuthState) => s.claims?.playerProfileId ?? null
