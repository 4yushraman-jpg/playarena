"use client"

import { useAuthGuard } from "@/hooks/use-auth-guard"
import { PageSkeleton } from "@/components/ui/loading-skeleton"

// GP-1 foundation: the player persona route group lives outside [orgSlug].
// It reuses the shared auth guard for silent-refresh hydration. Player-scope
// sessions have no organizationId, so the guard settles without redirecting
// into org selection. Full player UX is out of scope for GP-1.
export default function PlayerLayout({ children }: { children: React.ReactNode }) {
  const { isHydrating } = useAuthGuard()

  if (isHydrating) {
    return (
      <div className="flex min-h-svh items-start justify-center p-8">
        <div className="w-full max-w-5xl">
          <PageSkeleton />
        </div>
      </div>
    )
  }

  return <>{children}</>
}
