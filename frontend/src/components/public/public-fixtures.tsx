import Link from "next/link"
import { StatusBadge } from "@/components/ui/status-badge"
import { formatDateTime, formatScore } from "@/lib/format"
import type { PublicMatch } from "@/types/api/public"

interface PublicFixturesProps {
  matches: PublicMatch[]
  basePath: string
  // resultsOnly renders only concluded matches (the Results tab).
  resultsOnly?: boolean
  // upcomingOnly renders only matches still to come / in progress (the Fixtures
  // tab) — so Fixtures and Results never show the same match.
  upcomingOnly?: boolean
  emptyLabel?: string
}

const CONCLUDED = new Set(["completed", "walkover"])
const UPCOMING = new Set(["scheduled", "live", "postponed"])

export function PublicFixtures({ matches, basePath, resultsOnly, upcomingOnly, emptyLabel }: PublicFixturesProps) {
  const shown = resultsOnly
    ? matches.filter((m) => CONCLUDED.has(m.status))
    : upcomingOnly
      ? matches.filter((m) => UPCOMING.has(m.status))
      : matches

  if (shown.length === 0) {
    return (
      <p className="py-8 text-center text-sm text-muted-foreground">
        {emptyLabel ?? "No fixtures have been published yet."}
      </p>
    )
  }

  // Section by group label (group stage) or round name.
  const sections = new Map<string, PublicMatch[]>()
  for (const m of shown) {
    const key = m.group_label ? `Group ${m.group_label}` : m.round_name ?? "Matches"
    const arr = sections.get(key) ?? []
    arr.push(m)
    sections.set(key, arr)
  }

  return (
    <div className="space-y-5">
      {[...sections.entries()].map(([section, group]) => (
        <section key={section}>
          <h2 className="mb-2 text-xs font-semibold uppercase tracking-wide text-muted-foreground">
            {section}
          </h2>
          <ul className="divide-y divide-border rounded-lg border border-border">
            {group.map((m) => {
              const concluded = CONCLUDED.has(m.status)
              const homeWon = m.winner_name && m.winner_name === m.home.name
              const awayWon = m.winner_name && m.winner_name === m.away.name
              return (
                <li key={m.id}>
                  <Link
                    href={`${basePath}/matches/${m.id}`}
                    className="flex items-center gap-3 px-3 py-3 transition-colors hover:bg-accent/40 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-ring"
                  >
                    <div className="min-w-0 flex-1 space-y-0.5">
                      <p className="truncate text-sm">
                        <span className={homeWon ? "font-semibold" : ""}>{m.home.name}</span>
                        <span className="px-1.5 text-muted-foreground">vs</span>
                        <span className={awayWon ? "font-semibold" : ""}>{m.away.name}</span>
                      </p>
                      {m.scheduled_at && (
                        <p className="truncate text-xs text-muted-foreground">
                          {formatDateTime(m.scheduled_at)}
                          {m.venue ? ` · ${m.venue}` : ""}
                        </p>
                      )}
                    </div>
                    {m.is_walkover ? (
                      <span className="shrink-0 text-sm font-semibold text-amber-600 dark:text-amber-400">W/O</span>
                    ) : concluded ? (
                      <span className="shrink-0 tabular-nums text-sm font-semibold">
                        {formatScore(m.home_score, m.away_score)}
                      </span>
                    ) : null}
                    <StatusBadge status={m.status as never} />
                  </Link>
                </li>
              )
            })}
          </ul>
        </section>
      ))}
    </div>
  )
}
