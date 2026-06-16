import Link from "next/link"
import type { PublicMatch } from "@/types/api/public"

// PublicBracket renders the knockout graph as round columns. It uses only the
// next_match wiring already present in each match; group-stage matches (those
// with a group_label) are excluded — they belong to the standings view.
export function PublicBracket({ matches, basePath }: { matches: PublicMatch[]; basePath: string }) {
  const knockout = matches.filter((m) => !m.group_label && m.round_number != null)
  if (knockout.length === 0) {
    return <p className="py-8 text-center text-sm text-muted-foreground">No bracket has been published yet.</p>
  }

  // Group by round number, ascending → columns left (early) to right (final).
  const rounds = new Map<number, PublicMatch[]>()
  for (const m of knockout) {
    const r = m.round_number as number
    const arr = rounds.get(r) ?? []
    arr.push(m)
    rounds.set(r, arr)
  }
  const ordered = [...rounds.entries()].sort((a, b) => a[0] - b[0])

  return (
    <div className="overflow-x-auto pb-2">
      <div className="flex min-w-max gap-4">
        {ordered.map(([round, group]) => (
          <div key={round} className="flex min-w-[180px] flex-col gap-3">
            <h2 className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
              {group[0]?.round_name ?? `Round ${round}`}
            </h2>
            <div className="flex flex-1 flex-col justify-around gap-3">
              {group.map((m) => (
                <Link
                  key={m.id}
                  href={`${basePath}/matches/${m.id}`}
                  className="rounded-lg border border-border p-2 text-sm transition-colors hover:bg-accent/40 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                >
                  <BracketSide name={m.home.name} won={!!m.winner_name && m.winner_name === m.home.name} score={m.status === "completed" ? m.home_score : null} />
                  <div className="my-1 border-t border-dashed border-border" />
                  <BracketSide name={m.away.name} won={!!m.winner_name && m.winner_name === m.away.name} score={m.status === "completed" ? m.away_score : null} />
                  {m.is_walkover && <p className="mt-1 text-[10px] font-medium uppercase text-amber-600 dark:text-amber-400">Walkover</p>}
                </Link>
              ))}
            </div>
          </div>
        ))}
      </div>
    </div>
  )
}

function BracketSide({ name, won, score }: { name: string; won: boolean; score: number | null }) {
  return (
    <div className="flex items-center justify-between gap-2">
      <span className={won ? "truncate font-semibold" : "truncate"}>{name}</span>
      {score != null && <span className="shrink-0 tabular-nums text-muted-foreground">{score}</span>}
    </div>
  )
}
