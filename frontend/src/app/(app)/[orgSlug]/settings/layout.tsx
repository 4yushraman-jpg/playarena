"use client"

import Link from "next/link"
import { useParams, usePathname } from "next/navigation"
import { UserIcon, LockIcon, BellIcon } from "lucide-react"
import { cn } from "@/lib/utils"

interface SettingsNavItem {
  label: string
  href: (slug: string) => string
  icon: React.ElementType
}

const NAV_ITEMS: SettingsNavItem[] = [
  { label: "Profile", href: (s) => `/${s}/settings/profile`, icon: UserIcon },
  { label: "Security", href: (s) => `/${s}/settings/security`, icon: LockIcon },
  { label: "Notifications", href: (s) => `/${s}/settings/notifications`, icon: BellIcon },
]

export default function SettingsLayout({ children }: { children: React.ReactNode }) {
  const params = useParams<{ orgSlug: string }>()
  const orgSlug = params.orgSlug
  const pathname = usePathname()

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-semibold tracking-tight">Settings</h1>
        <p className="text-sm text-muted-foreground">
          Manage your account preferences and security.
        </p>
      </div>

      <div className="flex flex-col gap-6 lg:flex-row lg:gap-10">
        {/* Settings nav — vertical on desktop, horizontal tabs on mobile */}
        <nav
          aria-label="Settings navigation"
          className="flex shrink-0 gap-1 overflow-x-auto lg:w-44 lg:flex-col"
        >
          {NAV_ITEMS.map((item) => {
            const href = item.href(orgSlug)
            const isActive = pathname === href || pathname.startsWith(`${href}/`)
            return (
              <Link
                key={href}
                href={href}
                aria-current={isActive ? "page" : undefined}
                className={cn(
                  "flex items-center gap-2 whitespace-nowrap rounded-lg px-3 py-2 text-sm transition-colors",
                  isActive
                    ? "bg-accent text-accent-foreground font-medium"
                    : "text-muted-foreground hover:bg-muted hover:text-foreground",
                )}
              >
                <item.icon className="size-4 shrink-0" />
                {item.label}
              </Link>
            )
          })}
        </nav>

        {/* Settings content */}
        <div className="min-w-0 flex-1">{children}</div>
      </div>
    </div>
  )
}
