"use client"

import { CheckCircle2Icon, FlagIcon, OctagonAlertIcon } from "lucide-react"
import { cn } from "@/lib/utils"
import type { CompletionReadiness } from "@/lib/scoring/completion-gate"

interface CompletionGateBarProps {
  readiness: CompletionReadiness
  /** Abandon needs the queue synced (no unsynced/failed) but not a winner. */
  canAbandon: boolean
  onComplete: () => void
  onAbandon: () => void
}

/**
 * Terminal-action bar. "End & complete" is ENABLED only when the gate is
 * satisfied — match live, zero unsynced events, no permanent failures, and a
 * SETTLED authoritative score — so accidental or partial completion is
 * impossible. Abandon is a separate, gated, destructive secondary action.
 */
export function CompletionGateBar({ readiness, canAbandon, onComplete, onAbandon }: CompletionGateBarProps) {
  const { ready, reason } = readiness

  return (
    <div className="space-y-1.5">
      <button
        type="button"
        onClick={onComplete}
        disabled={!ready}
        aria-label="End and complete match"
        className={cn(
          "flex min-h-12 w-full items-center justify-center gap-2 rounded-xl px-4 text-base font-semibold transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:pointer-events-none disabled:opacity-50",
          ready
            ? "bg-foreground text-background hover:bg-foreground/90"
            : "border-2 border-border bg-card text-muted-foreground",
        )}
      >
        {ready ? <CheckCircle2Icon className="size-5" /> : <FlagIcon className="size-5" />}
        End &amp; complete
      </button>

      {!ready && reason && (
        <p className="text-center text-xs text-muted-foreground" role="status">
          {reason}
        </p>
      )}

      <button
        type="button"
        onClick={onAbandon}
        disabled={!canAbandon}
        aria-label="Abandon match"
        className="flex min-h-11 w-full items-center justify-center gap-1.5 rounded-xl px-4 text-sm font-medium text-destructive transition-colors hover:bg-destructive/10 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:pointer-events-none disabled:opacity-40"
      >
        <OctagonAlertIcon className="size-4" />
        Abandon match
      </button>
    </div>
  )
}
