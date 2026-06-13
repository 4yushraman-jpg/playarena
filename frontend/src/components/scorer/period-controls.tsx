"use client"

import { FlagOffIcon, PlayCircleIcon } from "lucide-react"

interface PeriodControlsProps {
  period: number
  disabled: boolean
  onEndHalf: () => void
  onStartNextHalf: () => void
}

/**
 * Period (half) transitions. Each emits a half_ended / half_started lifecycle
 * event (auditable, append-only) and advances the advisory clock. Two explicit
 * buttons — no ambiguous single toggle — so a referee can't mistake the state.
 */
export function PeriodControls({ period, disabled, onEndHalf, onStartNextHalf }: PeriodControlsProps) {
  return (
    <div className="grid grid-cols-2 gap-2">
      <button
        type="button"
        onClick={onEndHalf}
        disabled={disabled}
        className="flex min-h-12 items-center justify-center gap-1.5 rounded-xl border-2 border-border bg-card px-3 text-sm font-semibold transition-colors hover:bg-accent focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:pointer-events-none disabled:opacity-40"
      >
        <FlagOffIcon className="size-4" />
        End half {period}
      </button>
      <button
        type="button"
        onClick={onStartNextHalf}
        disabled={disabled}
        className="flex min-h-12 items-center justify-center gap-1.5 rounded-xl border-2 border-border bg-card px-3 text-sm font-semibold transition-colors hover:bg-accent focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:pointer-events-none disabled:opacity-40"
      >
        <PlayCircleIcon className="size-4" />
        Start half {period + 1}
      </button>
    </div>
  )
}
