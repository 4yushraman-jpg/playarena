"use client"

import { useEffect } from "react"
import { useRouter } from "next/navigation"
import { useAuthStore } from "@/stores/auth.store"
import { authApi } from "@/lib/api/auth"
import { extractApiError } from "@/lib/api-error"
import { toast } from "sonner"

export default function OrgSelectPage() {
  const router = useRouter()
  const { pendingOrgSelection, clearPendingOrgSelection, setSession, setOrgSlug, orgSlug } = useAuthStore()

  // Guard: redirect to login if no pending org selection AND no org has been chosen yet.
  // We check orgSlug because clearPendingOrgSelection() fires before router.push(/{slug})
  // in the success path — if we only checked pendingOrgSelection we would race-redirect
  // to /login immediately after a successful selection.
  useEffect(() => {
    if ((!pendingOrgSelection || pendingOrgSelection.length === 0) && !orgSlug) {
      router.replace("/login")
    }
  }, [pendingOrgSelection, orgSlug, router])

  // Render nothing if the list has been cleared (either because selection succeeded
  // and router.push is in flight, or because we landed here directly).
  if (!pendingOrgSelection || pendingOrgSelection.length === 0) return null

  async function selectOrg(orgId: string, orgSlug: string) {
    // Re-login is not needed: we store the email/password temporarily
    // in sessionStorage only during the org-picker flow.
    // Instead, call /auth/login again with organization_id in the request.
    // To avoid passing credentials around, we retrieve them from sessionStorage.
    const email = sessionStorage.getItem("pa_pending_email") ?? ""
    const password = sessionStorage.getItem("pa_pending_password") ?? ""

    if (!email || !password) {
      // Credentials expired from session — restart login
      clearPendingOrgSelection()
      router.push("/login")
      return
    }

    try {
      const { data } = await authApi.login({ email, password, organization_id: orgId })
      sessionStorage.removeItem("pa_pending_email")
      sessionStorage.removeItem("pa_pending_password")
      clearPendingOrgSelection()
      setSession(data)
      setOrgSlug(orgSlug)
      router.push(`/${orgSlug}`)
    } catch (err: unknown) {
      toast.error(extractApiError(err))
    }
  }

  return (
    <div className="rounded-xl border border-border bg-card p-8 shadow-sm space-y-6">
      <div className="space-y-1">
        <h1 className="text-xl font-semibold">Choose an organization</h1>
        <p className="text-sm text-muted-foreground">
          Your account belongs to multiple organizations. Select one to continue.
        </p>
      </div>

      <ul className="space-y-2">
        {pendingOrgSelection.map((org) => (
          <li key={org.id}>
            <button
              onClick={() => selectOrg(org.id, org.slug)}
              className="w-full rounded-lg border border-border bg-background px-4 py-3 text-left text-sm transition-colors hover:bg-muted focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            >
              <p className="font-medium">{org.name}</p>
              <p className="text-xs text-muted-foreground">/{org.slug}</p>
            </button>
          </li>
        ))}
      </ul>
    </div>
  )
}
