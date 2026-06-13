"use client"

import { CheckCircle2Icon, FlagIcon } from "lucide-react"
import { cn } from "@/lib/utils"
import type { CompletionReadiness } from "@/lib/scoring/completion-gate"

interface CompletionGateBarProps {
  readiness: CompletionReadiness
  onComplete: () => void
}

/**
 * Completion-gate foundations (FE-7BB). The "End & complete" affordance is
 * ENABLED only when the gate is satisfied — match live, zero unsynced events,
 * no permanent failures, and the authoritative score loaded. Until then it is
 * disabled with the blocking reason. (The actual completion flow lands in
 * FE-7BC; this proves the integrity gate that flow will sit behind.)
 */
export function CompletionGateBar({ readiness, onComplete }: CompletionGateBarProps) {
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
    </div>
  )
}
