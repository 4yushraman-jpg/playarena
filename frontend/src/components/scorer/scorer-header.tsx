"use client"

import { ChevronLeftIcon } from "lucide-react"
import { StatusBadge } from "@/components/ui/status-badge"
import { cn } from "@/lib/utils"
import type { MatchStatus } from "@/types/api/matches"

interface ScorerHeaderProps {
  matchLabel: string
  status: MatchStatus
  period: number | null
  onExit: () => void
}

/**
 * Top bar of the full-bleed scorer: exit, match label, status, period, and a
 * clock placeholder (the live clock arrives in FE-7BC — it is advisory and
 * never affects the score). The LIVE indicator gives an at-a-glance state.
 */
export function ScorerHeader({ matchLabel, status, period, onExit }: ScorerHeaderProps) {
  const isLive = status === "live"
  return (
    <header className="sticky top-0 z-10 flex items-center gap-2 border-b border-border bg-background/95 px-3 py-2 backdrop-blur supports-[backdrop-filter]:bg-background/80">
      <button
        type="button"
        onClick={onExit}
        aria-label="Exit scorer"
        className="-ml-1 flex size-11 shrink-0 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-accent hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
      >
        <ChevronLeftIcon className="size-5" />
      </button>

      <div className="min-w-0 flex-1">
        <p className="truncate text-sm font-medium" title={matchLabel}>
          {matchLabel}
        </p>
        <div className="mt-0.5 flex items-center gap-1.5 text-xs text-muted-foreground">
          <span aria-label="Clock placeholder" className="tabular-nums">
            --:--
          </span>
          <span aria-hidden="true">·</span>
          <span>{period != null ? `Half ${period}` : "Half —"}</span>
        </div>
      </div>

      {isLive && (
        <span className="flex shrink-0 items-center gap-1.5 rounded-full bg-green-50 px-2 py-0.5 text-xs font-semibold text-green-700 dark:bg-green-950/40 dark:text-green-300">
          <span className={cn("size-1.5 rounded-full bg-green-500", "animate-pulse")} aria-hidden="true" />
          LIVE
        </span>
      )}
      {!isLive && <StatusBadge status={status} />}
    </header>
  )
}
