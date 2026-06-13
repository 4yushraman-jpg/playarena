"use client"

import { cn } from "@/lib/utils"
import type { MatchStatus } from "@/types/api/matches"

interface ScoreboardProps {
  homeName: string
  awayName: string
  // null = not yet known (authoritative score still resolving). We never show a
  // placeholder 0 for a live match — a wrong score, even transiently, is worse
  // than an honest "syncing" dash.
  homeScore: number | null
  awayScore: number | null
  status: MatchStatus
  isWalkover: boolean
}

/**
 * The dominant element on the scorer screen: large, high-contrast,
 * tabular-numeral scores. Read-only. The numbers shown are the authoritative
 * server score (GET /score). Sides are distinguished by name + position, never
 * colour alone (daylight + colourblind safe).
 */
export function Scoreboard({
  homeName,
  awayName,
  homeScore,
  awayScore,
  status,
  isWalkover,
}: ScoreboardProps) {
  const known = homeScore != null && awayScore != null
  const isCompleted = status === "completed"
  const homeWon = isCompleted && known && homeScore > awayScore
  const awayWon = isCompleted && known && awayScore > homeScore

  return (
    <section
      aria-label="Scoreboard"
      className="grid grid-cols-[1fr_auto_1fr] items-center gap-2 sm:gap-4"
    >
      <ScoreSide name={homeName} score={homeScore} won={homeWon} align="right" />

      <div className="flex flex-col items-center px-1">
        <span className="text-xl font-medium text-muted-foreground sm:text-2xl" aria-hidden="true">
          –
        </span>
        {isWalkover && (
          <span className="mt-1 whitespace-nowrap text-[10px] font-semibold uppercase tracking-wide text-amber-600 dark:text-amber-400">
            Walkover
          </span>
        )}
      </div>

      <ScoreSide name={awayName} score={awayScore} won={awayWon} align="left" />
    </section>
  )
}

function ScoreSide({
  name,
  score,
  won,
  align,
}: {
  name: string
  score: number | null
  won: boolean
  align: "left" | "right"
}) {
  return (
    <div className={cn("min-w-0", align === "right" ? "text-right" : "text-left")}>
      <p
        className={cn(
          "truncate text-sm font-semibold uppercase tracking-wide sm:text-base",
          won ? "text-primary" : "text-muted-foreground",
        )}
        title={name}
      >
        {name}
      </p>
      <p
        className={cn(
          "mt-1 text-6xl font-bold tabular-nums leading-none sm:text-7xl",
          won ? "text-primary" : "text-foreground",
        )}
        aria-label={score == null ? `${name} score syncing` : `${name} score ${score}`}
      >
        {score == null ? <span className="text-muted-foreground/50">–</span> : score}
      </p>
    </div>
  )
}
