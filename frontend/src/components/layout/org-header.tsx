"use client"

import { useRouter } from "next/navigation"
import { MenuIcon, MoonIcon, SunIcon, LogOutIcon } from "lucide-react"
import { useTheme } from "next-themes"
import { Button } from "@/components/ui/button"
import { useUIStore } from "@/stores/ui.store"
import { useAuthStore } from "@/stores/auth.store"
import { tokenManager } from "@/lib/api/client"
import { authApi } from "@/lib/api/auth"
import { getQueryClient } from "@/lib/api/query-client"

interface OrgHeaderProps {
  orgSlug: string
}

// eslint-disable-next-line @typescript-eslint/no-unused-vars -- orgSlug prop reserved for FE-4 breadcrumbs
export function OrgHeader({ orgSlug: _orgSlug }: OrgHeaderProps) {
  const router = useRouter()
  const toggleSidebar = useUIStore((s) => s.toggleSidebar)
  const { resolvedTheme, setTheme } = useTheme()
  const { claims, clearSession } = useAuthStore()

  async function handleLogout() {
    const refreshToken = tokenManager.getRefreshToken()
    if (refreshToken) {
      await authApi.logout({ refresh_token: refreshToken }).catch(() => {})
    }
    // Clear cached org data before clearing auth state so the next user on
    // this device cannot briefly see stale data from the previous session.
    getQueryClient().clear()
    clearSession()
    router.push("/login")
  }

  return (
    <header className="sticky top-0 z-20 flex h-14 items-center gap-3 border-b border-border bg-background/80 px-4 backdrop-blur-sm">
      <Button
        variant="ghost"
        size="icon-sm"
        onClick={toggleSidebar}
        aria-label="Toggle sidebar"
      >
        <MenuIcon />
      </Button>

      <div className="flex-1" />

      {/* Theme toggle */}
      <Button
        variant="ghost"
        size="icon-sm"
        onClick={() => setTheme(resolvedTheme === "dark" ? "light" : "dark")}
        aria-label="Toggle theme"
      >
        {resolvedTheme === "dark" ? <SunIcon /> : <MoonIcon />}
      </Button>

      {/* User info + logout */}
      {claims && (
        <div className="flex items-center gap-2">
          <span className="text-sm text-muted-foreground hidden sm:block">{claims.email}</span>
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
