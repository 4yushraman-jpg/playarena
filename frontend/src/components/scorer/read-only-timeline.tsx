"use client"

import { useMemo } from "react"
import { cn } from "@/lib/utils"
import { cancelledEventIds, scoreContribution, type ScoringMatch } from "@/lib/scoring/engine"
import { eventKind, eventLabel } from "./event-labels"
import type { MatchEvent } from "@/types/api/match-events"

interface ReadOnlyTimelineProps {
  events: MatchEvent[]
  match: ScoringMatch
  resolveName: (teamId: string | null, playerId: string | null) => string
}

/**
 * Append-only event history, most-recent-first. Read-only. Cancelled events
 * (targets of a score_correction) are struck through and tagged so the full
 * audit trail is visible — nothing is hidden. Per-event point deltas use the
 * exact same engine as the score fold.
 */
export function ReadOnlyTimeline({ events, match, resolveName }: ReadOnlyTimelineProps) {
  const cancelled = useMemo(() => cancelledEventIds(events), [events])
  const ordered = useMemo(
    () => [...events].sort((a, b) => b.sequence_number - a.sequence_number),
    [events],
  )

  if (ordered.length === 0) {
    return (
      <p className="rounded-lg border border-dashed border-border px-3 py-6 text-center text-sm text-muted-foreground">
        No events recorded yet.
      </p>
    )
  }

  return (
    <ol className="space-y-1.5" aria-label="Event history">
      {ordered.map((e) => {
        const isCancelled = cancelled.has(e.id)
        const kind = eventKind(e.event_type)
        const { points, side } = scoreContribution(e, match)
        const hasName = !!(e.team_id || e.player_id)
        const name = hasName ? resolveName(e.team_id, e.player_id) : null

        return (
          <li
            key={e.id}
            className={cn(
              "flex items-center gap-2 rounded-lg border border-border/60 px-3 py-2 text-sm",
              isCancelled && "opacity-60",
              kind === "correction" && "border-amber-200 bg-amber-50/50 dark:border-amber-900 dark:bg-amber-950/20",
            )}
          >
            <span
              className="w-7 shrink-0 text-xs tabular-nums text-muted-foreground"
              aria-label={`Event ${e.sequence_number}`}
            >
              #{e.sequence_number}
            </span>

            <div className="min-w-0 flex-1">
              <p className={cn("truncate", isCancelled && "line-through")}>
                {name && <span className="font-medium">{name} · </span>}
                <span className={kind === "score" && !isCancelled ? "" : "text-muted-foreground"}>
                  {eventLabel(e.event_type)}
                </span>
              </p>
              {isCancelled && (
                <span className="text-[11px] font-medium uppercase tracking-wide text-amber-600 dark:text-amber-400">
                  Corrected
                </span>
              )}
            </div>

            {kind === "score" && points > 0 && (
              <span
                className={cn(
                  "shrink-0 rounded-md px-1.5 py-0.5 text-xs font-bold tabular-nums",
                  isCancelled
                    ? "text-muted-foreground"
                    : "bg-primary/10 text-primary",
                )}
                aria-label={`${points} point${points !== 1 ? "s" : ""} ${side}`}
              >
                +{points}
              </span>
            )}
          </li>
        )
      })}
    </ol>
  )
}
