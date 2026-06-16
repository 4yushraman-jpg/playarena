"use client"

import Link from "next/link"
import { usePathname } from "next/navigation"
import { cn } from "@/lib/utils"

interface PublicTabsProps {
  basePath: string // /t/{org}/{tournament}
  format: string
}

/**
 * Read-only tab navigation for the public tournament surface. Uses native links
 * (keyboard-accessible) with aria-current on the active tab. The bracket tab is
 * shown only for knockout-style formats.
 */
export function PublicTabs({ basePath, format }: PublicTabsProps) {
  const pathname = usePathname()
  const showBracket = format === "knockout" || format === "group_knockout"

  const tabs = [
    { href: basePath, label: "Overview" },
    { href: `${basePath}/fixtures`, label: "Fixtures" },
    ...(showBracket ? [{ href: `${basePath}/bracket`, label: "Bracket" }] : []),
    { href: `${basePath}/standings`, label: "Standings" },
    { href: `${basePath}/results`, label: "Results" },
  ]

  return (
    <nav aria-label="Tournament sections" className="-mb-px flex gap-1 overflow-x-auto">
      {tabs.map((tab) => {
        const active = pathname === tab.href
        return (
          <Link
            key={tab.href}
            href={tab.href}
            aria-current={active ? "page" : undefined}
            className={cn(
              "shrink-0 border-b-2 px-3 py-2 text-sm font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring",
              active
                ? "border-primary text-primary"
                : "border-transparent text-muted-foreground hover:text-foreground",
            )}
          >
            {tab.label}
          </Link>
        )
      })}
    </nav>
  )
}
