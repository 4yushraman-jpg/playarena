import { StatusBadge } from "@/components/ui/status-badge"
import { formatDateTime } from "@/lib/format"
import type { PublicMatchDetail as MatchDetail } from "@/types/api/public"

const CONCLUDED = new Set(["completed", "walkover"])

export function PublicMatchDetail({ match }: { match: MatchDetail }) {
  const concluded = CONCLUDED.has(match.status)
  const homeWon = match.winner_name && match.winner_name === match.home.name
  const awayWon = match.winner_name && match.winner_name === match.away.name

  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-center gap-3 text-sm text-muted-foreground">
        <StatusBadge status={match.status as never} />
        {match.round_name && <span>{match.round_name}</span>}
        {match.group_label && <span>Group {match.group_label}</span>}
      </div>

      {/* Scoreboard */}
      <div className="rounded-xl border border-border py-6">
        <div className="grid grid-cols-[1fr_auto_1fr] items-center gap-3 px-4">
          <p className={`truncate text-right text-base font-semibold ${homeWon ? "text-primary" : ""}`}>
            {match.home.name}
          </p>
          <div className="text-center">
            {match.is_walkover ? (
              <span className="text-xl font-bold uppercase text-amber-600 dark:text-amber-400">W/O</span>
            ) : concluded ? (
              <span className="tabular-nums text-2xl font-bold">
                {match.home_score} <span className="text-muted-foreground">–</span> {match.away_score}
              </span>
            ) : (
              <span className="text-sm uppercase tracking-wide text-muted-foreground">vs</span>
            )}
          </div>
          <p className={`truncate text-left text-base font-semibold ${awayWon ? "text-primary" : ""}`}>
            {match.away.name}
          </p>
        </div>
        {match.winner_name && (
          <p className="mt-3 text-center text-xs font-medium text-primary">Winner: {match.winner_name}</p>
        )}
      </div>

      {(match.scheduled_at || match.venue) && (
        <dl className="grid gap-2 text-sm sm:grid-cols-2">
          {match.scheduled_at && (
            <div>
              <dt className="text-xs text-muted-foreground">Scheduled</dt>
              <dd className="font-medium">{formatDateTime(match.scheduled_at)}</dd>
            </div>
          )}
          {match.venue && (
            <div>
              <dt className="text-xs text-muted-foreground">Venue</dt>
              <dd className="font-medium">{match.venue}</dd>
            </div>
          )}
        </dl>
      )}

      {match.timeline.length > 0 && (
        <section>
          <h2 className="mb-2 text-sm font-semibold">Timeline</h2>
          <ol className="space-y-1">
            {match.timeline.map((e) => (
              <li key={e.sequence} className="flex items-center gap-2 text-sm">
                <span className="w-10 shrink-0 tabular-nums text-xs text-muted-foreground">
                  {e.period ? `P${e.period}` : `#${e.sequence}`}
                </span>
                <span className="font-medium">{formatEvent(e.event_type)}</span>
                {e.actor_name && <span className="truncate text-muted-foreground">· {e.actor_name}</span>}
              </li>
            ))}
          </ol>
        </section>
      )}
    </div>
  )
}

function formatEvent(t: string): string {
  return t.replace(/_/g, " ").replace(/\b\w/g, (c) => c.toUpperCase())
}
