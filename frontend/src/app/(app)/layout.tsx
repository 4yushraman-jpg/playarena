"use client"

import { useAuthGuard } from "@/hooks/use-auth-guard"
import { PageSkeleton } from "@/components/ui/loading-skeleton"

export default function AppLayout({ children }: { children: React.ReactNode }) {
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
