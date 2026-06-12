"use client"

import { useEffect } from "react"
import { useRouter } from "next/navigation"
import { useAuthGuard } from "@/hooks/use-auth-guard"
import { PageSkeleton } from "@/components/ui/loading-skeleton"

// Root page: resolve the session (possibly via silent token refresh) and redirect.
//   - session restored + orgSlug known → /{orgSlug}
//   - session restored, orgSlug unknown → /login (unlikely but safe fallback)
//   - no refresh token → /login
//
// Without calling useAuthGuard here, a user who bookmarks "/" would be kicked to
// /login every time even with a valid refresh token, because Zustand resets on
// each page load and orgSlug is not known until hydration runs.
export default function RootPage() {
  const router = useRouter()
  const { isHydrating, claims, orgSlug } = useAuthGuard()

  useEffect(() => {
    if (isHydrating) return
    if (orgSlug) {
      router.replace(`/${orgSlug}`)
    } else if (claims?.role === "onboarding") {
      router.replace("/onboarding")
    } else {
      router.replace("/login")
    }
  }, [claims?.role, isHydrating, orgSlug, router])

  return (
    <div className="flex min-h-svh items-start justify-center p-8">
      <div className="w-full max-w-5xl">
        <PageSkeleton />
      </div>
    </div>
  )
}
