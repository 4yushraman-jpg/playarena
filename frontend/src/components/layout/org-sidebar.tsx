"use client"

import { useMemo } from "react"
import Link from "next/link"
import { usePathname } from "next/navigation"
import {
  HomeIcon,
  UsersIcon,
  ShieldIcon,
  TrophyIcon,
  SwordsIcon,
  BarChart2Icon,
  BellIcon,
  WebhookIcon,
  ImageIcon,
  SettingsIcon,
} from "lucide-react"
import { cn } from "@/lib/utils"
import { useUIStore } from "@/stores/ui.store"
import { useAuthStore, selectRole } from "@/stores/auth.store"

interface NavItem {
  label: string
  href: string
  icon: React.ElementType
}

function buildNav(orgSlug: string): NavItem[] {
  const base = `/${orgSlug}`
  return [
    { label: "Dashboard",     href: base,                    icon: HomeIcon },
    { label: "Players",       href: `${base}/players`,       icon: UsersIcon },
    { label: "Teams",         href: `${base}/teams`,         icon: ShieldIcon },
    { label: "Tournaments",   href: `${base}/tournaments`,   icon: TrophyIcon },
    { label: "Matches",       href: `${base}/matches`,       icon: SwordsIcon },
    { label: "Rankings",      href: `${base}/rankings`,      icon: BarChart2Icon },
    { label: "Notifications", href: `${base}/notifications`, icon: BellIcon },
    { label: "Webhooks",      href: `${base}/webhooks`,      icon: WebhookIcon },
    { label: "Media",         href: `${base}/media`,         icon: ImageIcon },
    { label: "Settings",      href: `${base}/settings`,      icon: SettingsIcon },
  ]
}

interface OrgSidebarProps {
  orgSlug: string
}

export function OrgSidebar({ orgSlug }: OrgSidebarProps) {
  const pathname = usePathname()
  const { sidebarOpen, setSidebarOpen } = useUIStore()
  const role = useAuthStore(selectRole)
  const nav = useMemo(() => buildNav(orgSlug), [orgSlug])

  function handleNavClick() {
    // Close the sidebar drawer after navigation on mobile.
    if (typeof window !== "undefined" && window.innerWidth < 1024) {
      setSidebarOpen(false)
    }
  }

  return (
    <aside
      aria-label="Primary navigation"
      className={cn(
        "fixed inset-y-0 left-0 z-30 flex w-60 flex-col border-r border-sidebar-border bg-sidebar transition-transform duration-200",
        !sidebarOpen && "-translate-x-full",
      )}
    >
      {/* Brand */}
      <div className="flex h-14 items-center gap-2 border-b border-sidebar-border px-4">
        <div className="flex size-7 items-center justify-center rounded-lg bg-sidebar-primary text-sidebar-primary-foreground text-xs font-bold select-none">
          PA
        </div>
        <span className="text-sm font-semibold text-sidebar-foreground truncate">PlayArena</span>
      </div>

      {/* Org slug pill */}
      <div className="px-3 py-2">
        <div className="rounded-md bg-sidebar-accent px-2.5 py-1">
          <p className="text-xs font-medium text-sidebar-accent-foreground truncate">/{orgSlug}</p>
        </div>
      </div>

      {/* Nav */}
      <nav className="flex-1 overflow-y-auto px-2 py-1 space-y-0.5">
        {nav.map((item) => {
          const isActive = pathname === item.href || pathname.startsWith(`${item.href}/`)
          return (
            <Link
              key={item.href}
              href={item.href}
              onClick={handleNavClick}
              aria-current={isActive ? "page" : undefined}
              className={cn(
                "flex items-center gap-2.5 rounded-md px-2.5 py-1.5 text-sm transition-colors",
                isActive
                  ? "bg-sidebar-accent text-sidebar-accent-foreground font-medium"
                  : "text-sidebar-foreground hover:bg-sidebar-accent/60 hover:text-sidebar-accent-foreground",
              )}
            >
              <item.icon className="size-4 shrink-0" />
              {item.label}
            </Link>
          )
        })}
      </nav>

      {/* Role indicator */}
      {role && (
        <div className="border-t border-sidebar-border px-4 py-3">
          <p className="text-xs text-sidebar-foreground/60 capitalize">{role.replace(/_/g, " ")}</p>
        </div>
      )}
    </aside>
  )
}
