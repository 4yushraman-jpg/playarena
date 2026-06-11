"use client"

import { CheckIcon } from "lucide-react"
import { cn } from "@/lib/utils"
import type { TournamentStatus } from "@/types/api/tournaments"

// ── Ordered lifecycle steps ────────────────────────────────────────────────────

interface Step {
  status: TournamentStatus
  label: string
}

export const TIMELINE_STEPS: Step[] = [
  { status: "draft", label: "Draft" },
  { status: "registration_open", label: "Open" },
  { status: "registration_closed", label: "Closed" },
  { status: "ongoing", label: "In Progress" },
  { status: "completed", label: "Completed" },
]

export const STEP_ORDER: Record<TournamentStatus, number> = {
  draft: 0,
  registration_open: 1,
  registration_closed: 2,
  ongoing: 3,
  completed: 4,
  cancelled: -1,
}

/**
 * Compact summary for narrow viewports, e.g. "Step 2 of 5 · Open".
 * Exported for tests; returns null for the cancelled state.
 */
export function getTimelineSummary(status: TournamentStatus): string | null {
  const order = STEP_ORDER[status]
  if (order < 0) return null
  return `Step ${order + 1} of ${TIMELINE_STEPS.length} · ${TIMELINE_STEPS[order].label}`
}

// ── Component ─────────────────────────────────────────────────────────────────

interface TournamentTimelineProps {
  status: TournamentStatus
}

export function TournamentTimeline({ status }: TournamentTimelineProps) {
  if (status === "cancelled") {
    return (
      <div
        className="flex items-center gap-2 rounded-lg border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700 dark:border-red-800 dark:bg-red-950/40 dark:text-red-300"
        role="status"
      >
        <span className="text-xs font-medium uppercase tracking-wide">Cancelled</span>
        <span className="text-xs text-muted-foreground">This tournament was cancelled.</span>
      </div>
    )
  }

  const currentOrder = STEP_ORDER[status]

  return (
    <nav aria-label="Tournament lifecycle" className="w-full">
      {/* Compact variant — narrow viewports get a single readable line plus
          progress dots instead of five cramped columns. */}
      <div className="flex items-center justify-between gap-3 sm:hidden">
        <span className="text-sm font-medium text-foreground">
          {getTimelineSummary(status)}
        </span>
        <ol className="flex items-center gap-1.5" role="list">
          {TIMELINE_STEPS.map((step) => {
            const stepOrder = STEP_ORDER[step.status]
            const isCurrent = step.status === status
            return (
              <li key={step.status} aria-current={isCurrent ? "step" : undefined}>
                <span
                  className={cn(
                    "block size-2 rounded-full",
                    stepOrder < currentOrder && "bg-primary",
                    isCurrent && "bg-primary ring-2 ring-primary/30",
                    stepOrder > currentOrder && "bg-muted-foreground/25",
                  )}
                  aria-hidden="true"
                />
                <span className="sr-only">{step.label}</span>
              </li>
            )
          })}
        </ol>
      </div>

      {/* Full horizontal rail from `sm` up. */}
      <ol className="hidden items-center gap-0 sm:flex" role="list">
        {TIMELINE_STEPS.map((step, i) => {
          const stepOrder = STEP_ORDER[step.status]
          const isCompleted = stepOrder < currentOrder
          const isCurrent = step.status === status
          const isLast = i === TIMELINE_STEPS.length - 1

          return (
            <li
              key={step.status}
              className="flex flex-1 items-center"
              aria-current={isCurrent ? "step" : undefined}
            >
              {/* Step node */}
              <div className="flex flex-col items-center gap-1">
                <div
                  className={cn(
                    "flex size-7 items-center justify-center rounded-full border-2 text-xs font-semibold transition-colors",
                    isCompleted &&
                      "border-primary bg-primary text-primary-foreground",
                    isCurrent &&
                      "border-primary bg-primary/10 text-primary ring-2 ring-primary/20",
                    !isCompleted &&
                      !isCurrent &&
                      "border-muted-foreground/30 bg-background text-muted-foreground/50",
                  )}
                >
                  {isCompleted ? (
                    <CheckIcon className="size-3.5" />
                  ) : (
                    <span>{i + 1}</span>
                  )}
                </div>
                <span
                  className={cn(
                    "text-center text-xs font-medium leading-tight",
                    isCurrent && "text-primary",
                    isCompleted && "text-foreground",
                    !isCompleted && !isCurrent && "text-muted-foreground/50",
                  )}
                >
                  {step.label}
                </span>
              </div>

              {/* Connector */}
              {!isLast && (
                <div
                  className={cn(
                    "h-0.5 flex-1",
                    stepOrder < currentOrder ? "bg-primary" : "bg-muted",
                  )}
                  aria-hidden="true"
                />
              )}
            </li>
          )
        })}
      </ol>
    </nav>
  )
}
