"use client"

import { ChevronLeftIcon, PauseIcon, PlayIcon } from "lucide-react"
import { StatusBadge } from "@/components/ui/status-badge"
import type { MatchStatus } from "@/types/api/matches"

export interface HeaderClock {
  elapsedSeconds: number
  running: boolean
  onToggle: () => void
}

interface ScorerHeaderProps {
  matchLabel: string
  status: MatchStatus
  period: number | null
  /** Present in live-scoring mode — renders an interactive advisory clock.
   *  The clock NEVER affects the score (which is derived from the event log). */
  clock?: HeaderClock
  onExit: () => void
}

function formatClock(totalSeconds: number): string {
  const m = Math.floor(totalSeconds / 60)
  const s = totalSeconds % 60
  return `${String(m).padStart(2, "0")}:${String(s).padStart(2, "0")}`
}

/**
 * Top bar of the full-bleed scorer: exit, match label, status, period, and an
 * advisory clock (live mode). The clock is pausable for timeouts/halftime but
 * is purely informational — the result comes from the event log.
 */
export function ScorerHeader({ matchLabel, status, period, clock, onExit }: ScorerHeaderProps) {
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
          <span className="tabular-nums" aria-label="Match clock (advisory)">
            {clock ? formatClock(clock.elapsedSeconds) : "--:--"}
          </span>
          <span aria-hidden="true">·</span>
          <span>{period != null ? `Half ${period}` : "Half —"}</span>
        </div>
      </div>

      {clock && (
        <button
          type="button"
          onClick={clock.onToggle}
          aria-label={clock.running ? "Pause clock" : "Resume clock"}
          className="flex size-11 shrink-0 items-center justify-center rounded-md border border-border text-foreground transition-colors hover:bg-accent focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
        >
          {clock.running ? <PauseIcon className="size-4" /> : <PlayIcon className="size-4" />}
        </button>
      )}

      {isLive && (
        <span className="flex shrink-0 items-center gap-1.5 rounded-full bg-green-50 px-2 py-0.5 text-xs font-semibold text-green-700 dark:bg-green-950/40 dark:text-green-300">
          <span className="size-1.5 animate-pulse rounded-full bg-green-500" aria-hidden="true" />
          LIVE
        </span>
      )}
      {!isLive && <StatusBadge status={status} />}
    </header>
  )
}
