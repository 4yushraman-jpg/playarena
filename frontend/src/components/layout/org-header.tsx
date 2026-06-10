"use client"

import Link from "next/link"
import { useRouter } from "next/navigation"
import { MenuIcon, MoonIcon, SunIcon, LogOutIcon, BellIcon } from "lucide-react"
import { useTheme } from "next-themes"
import { Button } from "@/components/ui/button"
import { Badge } from "@/components/ui/badge"
import { OrgSwitcher } from "@/components/layout/org-switcher"
import { useUIStore } from "@/stores/ui.store"
import { useAuthStore } from "@/stores/auth.store"
import { tokenManager } from "@/lib/api/client"
import { authApi } from "@/lib/api/auth"
import { getQueryClient } from "@/lib/api/query-client"
import { useUnreadCount } from "@/hooks/use-unread-count"

interface OrgHeaderProps {
  orgSlug: string
}

export function OrgHeader({ orgSlug }: OrgHeaderProps) {
  const router = useRouter()
  const toggleSidebar = useUIStore((s) => s.toggleSidebar)
  const { resolvedTheme, setTheme } = useTheme()
  const { claims, clearSession } = useAuthStore()
  const { unreadCount } = useUnreadCount(orgSlug)

  async function handleLogout() {
    const refreshToken = tokenManager.getRefreshToken()
    if (refreshToken) {
      await authApi.logout({ refresh_token: refreshToken }).catch(() => {})
    }
    getQueryClient().clear()
    clearSession()
    router.push("/login")
  }

  return (
    <header className="sticky top-0 z-20 flex h-14 items-center gap-2 border-b border-border bg-background/80 px-4 backdrop-blur-sm">
      <Button
        variant="ghost"
        size="icon-sm"
        onClick={toggleSidebar}
        aria-label="Toggle sidebar"
      >
        <MenuIcon />
      </Button>

      {/* Org identity — switcher or static name */}
      <OrgSwitcher currentOrgSlug={orgSlug} />

      <div className="flex-1" />

      {/* Notification bell */}
      <Button variant="ghost" size="icon-sm" asChild>
        <Link
          href={`/${orgSlug}/notifications`}
          aria-label={`Notifications${unreadCount > 0 ? `, ${unreadCount} unread` : ""}`}
          className="relative"
        >
          <BellIcon />
          {unreadCount > 0 && (
            <Badge
              variant="default"
              className="absolute -right-1 -top-1 flex h-4 min-w-4 items-center justify-center rounded-full px-1 text-[10px] leading-none"
              aria-hidden="true"
            >
              {unreadCount > 9 ? "9+" : unreadCount}
            </Badge>
          )}
        </Link>
      </Button>

      {/* Theme toggle */}
      <Button
        variant="ghost"
        size="icon-sm"
        onClick={() => setTheme(resolvedTheme === "dark" ? "light" : "dark")}
        aria-label={`Switch to ${resolvedTheme === "dark" ? "light" : "dark"} mode`}
      >
        {resolvedTheme === "dark" ? <SunIcon /> : <MoonIcon />}
      </Button>

      {/* User info + logout */}
      {claims && (
        <div className="flex items-center gap-2">
          <span className="hidden text-sm text-muted-foreground sm:block">
            {claims.email}
          </span>
          <Button
            variant="ghost"
            size="icon-sm"
            onClick={handleLogout}
            aria-label="Sign out"
          >
            <LogOutIcon />
          </Button>
        </div>
      )}
    </header>
  )
}
