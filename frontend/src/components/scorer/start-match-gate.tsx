"use client"

import { ClockIcon } from "lucide-react"
import { formatDateTime } from "@/lib/format"

interface StartMatchGateProps {
  scheduledAt: string | null
}

/**
 * Shown for a scheduled match. FE-7BA is read-only, so this is informational —
 * there is no Start action here (starting a match is a write, introduced in
 * FE-7BB). It sets expectations: scoring opens once the match is live.
 */
export function StartMatchGate({ scheduledAt }: StartMatchGateProps) {
  return (
    <div className="flex flex-col items-center gap-3 rounded-xl border border-dashed border-border bg-muted/30 px-4 py-10 text-center">
      <div className="flex size-12 items-center justify-center rounded-full bg-muted">
        <ClockIcon className="size-6 text-muted-foreground" />
      </div>
      <div className="space-y-1">
        <p className="text-sm font-medium">This match hasn&apos;t started yet</p>
        <p className="text-xs text-muted-foreground">
          {scheduledAt
            ? `Scheduled for ${formatDateTime(scheduledAt)}.`
            : "No start time has been set."}
        </p>
        <p className="text-xs text-muted-foreground">
          The live scoreboard appears here once the match is underway.
        </p>
      </div>
    </div>
  )
}
