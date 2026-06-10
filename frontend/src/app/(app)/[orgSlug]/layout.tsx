"use client"

import { useEffect } from "react"
import { useParams } from "next/navigation"
import { OrgSidebar } from "@/components/layout/org-sidebar"
import { OrgHeader } from "@/components/layout/org-header"
import { useUIStore } from "@/stores/ui.store"
import { useNotificationStream } from "@/hooks/use-notification-stream"
import { cn } from "@/lib/utils"

export default function OrgLayout({ children }: { children: React.ReactNode }) {
  const params = useParams<{ orgSlug: string }>()
  const orgSlug = params.orgSlug
  const { sidebarOpen, setSidebarOpen } = useUIStore()

  // Default to closed on mobile; default to open on desktop.
  // Runs once on mount — the layout persists across child-route navigations
  // so this doesn't fire again on every page change.
  useEffect(() => {
    if (typeof window !== "undefined" && window.innerWidth < 1024) {
      setSidebarOpen(false)
    }
  }, [setSidebarOpen])

  useNotificationStream({ orgSlug })

  return (
    <div className="flex min-h-svh">
      {/* Mobile overlay — covers content behind the open sidebar on small screens. */}
      {sidebarOpen && (
        <div
          className="fixed inset-0 z-20 bg-black/40 lg:hidden"
          onClick={() => setSidebarOpen(false)}
          aria-hidden="true"
        />
      )}

      <OrgSidebar orgSlug={orgSlug} />

      {/*
        On desktop (lg+) the sidebar is always visible and we offset the main
        content with lg:ml-60. On mobile the sidebar is an overlay drawer so
        the main content spans full width.
      */}
      <div
        className={cn(
          "flex flex-1 flex-col transition-[margin] duration-200",
          sidebarOpen ? "lg:ml-60" : "",
        )}
      >
        <OrgHeader orgSlug={orgSlug} />
        <main className="flex-1 p-6">{children}</main>
      </div>
    </div>
  )
}
