"use client"

import { UsersIcon } from "lucide-react"

/**
 * Frontend scorer-ownership awareness (no backend lease). If the event log
 * contains events recorded by someone other than the current user, two people
 * may be scoring the same match — which would double-enter real-world actions.
 * We surface this loudly rather than let each scorer believe they're alone.
 */
export function ConcurrentScorerBanner() {
  return (
    <div
      role="alert"
      className="flex items-start gap-2.5 rounded-lg border border-red-300 bg-red-50 px-3 py-2.5 text-sm text-red-800 dark:border-red-900 dark:bg-red-950/30 dark:text-red-200"
    >
      <UsersIcon className="mt-0.5 size-4 shrink-0" />
      <div>
        <p className="font-medium">Another scorer is recording this match</p>
        <p className="text-xs opacity-90">
          Events from a different account are on this match. Coordinate so only one person scores —
          two scorers will double-count points.
        </p>
      </div>
    </div>
  )
}
