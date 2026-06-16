"use client"

import { useEffect, useSyncExternalStore } from "react"
import { notFound, useParams, useRouter } from "next/navigation"
import { isReservedSlug } from "@/lib/reserved-slugs"
import { OrgSidebar } from "@/components/layout/org-sidebar"
import { OrgHeader } from "@/components/layout/org-header"
import { useUIStore } from "@/stores/ui.store"
import { useAuthStore } from "@/stores/auth.store"
import { hasOrgContext } from "@/lib/permissions"
import { useNotificationStream } from "@/hooks/use-notification-stream"
import { cn } from "@/lib/utils"

// useSyncExternalStore subscribe/snapshot helpers for "(min-width: 1024px)" media query.
// Defined outside the component so they are referentially stable across renders.
const DESKTOP_MQL = "(min-width: 1024px)"

function subscribeDesktop(cb: () => void) {
  const mql = window.matchMedia(DESKTOP_MQL)
  mql.addEventListener("change", cb)
  return () => mql.removeEventListener("change", cb)
}

const getIsDesktop = () => window.matchMedia(DESKTOP_MQL).matches
const getIsDesktopServer = () => true // SSR/hydration safe default

export default function OrgLayout({ children }: { children: React.ReactNode }) {
  const params = useParams<{ orgSlug: string }>()
  const orgSlug = params.orgSlug
  // Defense in depth: a reserved segment (e.g. /me) must never resolve as an org.
  if (orgSlug && isReservedSlug(orgSlug)) {
    notFound()
  }
  const router = useRouter()
  const claims = useAuthStore((s) => s.claims)
  const isHydrating = useAuthStore((s) => s.isHydrating)
  // The org shell is the organizer application. A bare top-level path such as
  // /tournaments is captured by this [orgSlug] segment (orgSlug="tournaments"),
  // so without this guard an onboarding/no-org user would render the full
  // organizer shell while every org-scoped widget 403s. Onboarding and player
  // users have no org context and must be sent to the neutral /welcome landing.
  // Mirrors the backend RequireOrgScope boundary; platform admins (empty org,
  // scope="platform") are allowed through. See hasOrgContext.
  const orgAllowed = hasOrgContext(claims)
  useEffect(() => {
    // Only redirect once auth has hydrated and we know the user lacks org
    // context. Before hydration claims may be transiently null; the parent
    // (app)/layout shows a skeleton until then.
    if (!isHydrating && claims && !orgAllowed) {
      router.replace("/welcome")
    }
  }, [isHydrating, claims, orgAllowed, router])

  const { sidebarOpen, setSidebarOpen } = useUIStore()

  // Subscribes to matchMedia changes without calling setState inside an effect.
  // getIsDesktopServer returns true so SSR and initial hydration assume desktop.
  const isDesktop = useSyncExternalStore(subscribeDesktop, getIsDesktop, getIsDesktopServer)

  // Default to closed on mobile; default to open on desktop.
  useEffect(() => {
    if (typeof window !== "undefined" && window.innerWidth < 1024) {
      setSidebarOpen(false)
    }
  }, [setSidebarOpen])

  const isMobileDrawerOpen = sidebarOpen && !isDesktop

  // Escape key closes the mobile drawer.
  useEffect(() => {
    if (!isMobileDrawerOpen) return
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === "Escape") setSidebarOpen(false)
    }
    document.addEventListener("keydown", handleKeyDown)
    return () => document.removeEventListener("keydown", handleKeyDown)
  }, [isMobileDrawerOpen, setSidebarOpen])

  // Move focus into the sidebar when the mobile drawer opens.
  useEffect(() => {
    if (!isMobileDrawerOpen) return
    const sidebar = document.querySelector<HTMLElement>("[data-sidebar='true']")
    const firstFocusable = sidebar?.querySelector<HTMLElement>("a, button")
    firstFocusable?.focus()
  }, [isMobileDrawerOpen])

  useNotificationStream({ orgSlug, enabled: orgAllowed })

  // Don't flash the organizer shell for users without org context — render
  // nothing while the redirect to /welcome (above) takes effect. Authenticated
  // organizer/platform users fall straight through to the shell.
  if (claims && !orgAllowed) {
    return null
  }

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

      <OrgSidebar orgSlug={orgSlug} isDrawerMode={isMobileDrawerOpen} />

      {/*
        On desktop (lg+) the sidebar is always visible and we offset the main
        content with lg:ml-60. On mobile the sidebar is an overlay drawer so
        the main content spans full width. When the mobile drawer is open,
        main content is marked inert to trap focus inside the sidebar.
      */}
      <div
        {...(isMobileDrawerOpen ? { inert: true } : {})}
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
