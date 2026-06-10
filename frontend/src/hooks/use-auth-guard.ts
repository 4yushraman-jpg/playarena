"use client"

import { useEffect } from "react"
import { useRouter } from "next/navigation"
import { useAuthStore } from "@/stores/auth.store"
import { tokenManager } from "@/lib/api/client"
import { authApi } from "@/lib/api/auth"
import { orgsApi } from "@/lib/api/organizations"

/**
 * Runs on mount in protected layouts. Attempts a silent refresh using the
 * stored refresh token, then sets isHydrating = false. If no refresh token
 * exists the user is redirected to /login.
 */
export function useAuthGuard() {
  const { claims, hydrateClaims, clearSession, isHydrating, setHydrating, orgSlug, setOrgSlug } =
    useAuthStore()
  const router = useRouter()

  useEffect(() => {
    if (!isHydrating) return

    const refreshToken = tokenManager.getRefreshToken()

    if (!refreshToken) {
      clearSession()
      router.replace("/login")
      return
    }

    // Access token still valid — skip refresh, just verify with /auth/me
    const accessToken = tokenManager.getAccessToken()
    if (accessToken && claims) {
      const msUntilExpiry = claims.exp * 1000 - Date.now()
      if (msUntilExpiry > 60_000) {
        // Resolve org slug if not yet known
        if (!orgSlug) {
          orgsApi
            .list({ limit: 1 })
            .then(({ data }) => {
              const slug = data.organizations[0]?.slug
              if (slug) setOrgSlug(slug)
            })
            .catch(() => {})
            .finally(() => setHydrating(false))
        } else {
          setHydrating(false)
        }
        return
      }
    }

    // Attempt silent refresh via /auth/me (the 401 interceptor will refresh the token).
    // After me() resolves, the interceptor may have stored a new access token in
    // sessionStorage — hydrateClaims() decodes it and writes claims to the store.
    authApi
      .me()
      .then(() => {
        hydrateClaims()
        if (!orgSlug) {
          return orgsApi.list({ limit: 1 }).then(({ data }) => {
            const slug = data.organizations[0]?.slug
            if (slug) setOrgSlug(slug)
          })
        }
      })
      .then(() => setHydrating(false))
      .catch(() => {
        clearSession()
        router.replace("/login")
      })
  }, []) // eslint-disable-line react-hooks/exhaustive-deps

  return { isHydrating, claims, orgSlug }
}
