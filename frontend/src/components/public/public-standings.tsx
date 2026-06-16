import type { PublicStandingsResponse } from "@/types/api/public"

export function PublicStandings({ data }: { data: PublicStandingsResponse }) {
  if (data.standings.length === 0) {
    return <p className="py-8 text-center text-sm text-muted-foreground">Standings will appear once matches are played.</p>
  }
  return (
    <div className="overflow-x-auto rounded-lg border border-border">
      <table className="w-full text-sm">
        <caption className="sr-only">Tournament standings</caption>
        <thead>
          <tr className="border-b border-border bg-muted/40 text-left text-xs uppercase tracking-wide text-muted-foreground">
            <th scope="col" className="px-3 py-2 font-medium">#</th>
            <th scope="col" className="px-3 py-2 font-medium">Participant</th>
            <th scope="col" className="px-2 py-2 text-center font-medium" title="Played">P</th>
            <th scope="col" className="px-2 py-2 text-center font-medium" title="Won">W</th>
            <th scope="col" className="px-2 py-2 text-center font-medium" title="Lost">L</th>
            <th scope="col" className="px-2 py-2 text-center font-medium" title="Drawn">D</th>
            <th scope="col" className="px-2 py-2 text-center font-medium" title="Score difference">+/–</th>
            <th scope="col" className="px-3 py-2 text-center font-semibold">Pts</th>
          </tr>
        </thead>
        <tbody>
          {data.standings.map((row) => (
            <tr key={`${row.position}-${row.participant_name}`} className="border-b border-border last:border-0">
              <td className="px-3 py-2 tabular-nums text-muted-foreground">{row.position}</td>
              <td className="px-3 py-2">
                <span className="font-medium">{row.participant_name}</span>
                {row.disqualified && (
                  <span className="ml-2 rounded bg-red-50 px-1.5 py-0.5 text-[10px] font-medium uppercase text-red-700 dark:bg-red-950/40 dark:text-red-300">
                    Disqualified
                  </span>
                )}
              </td>
              <td className="px-2 py-2 text-center tabular-nums">{row.played}</td>
              <td className="px-2 py-2 text-center tabular-nums">{row.wins}</td>
              <td className="px-2 py-2 text-center tabular-nums">{row.losses}</td>
              <td className="px-2 py-2 text-center tabular-nums">{row.draws}</td>
              <td className="px-2 py-2 text-center tabular-nums">{row.score_difference > 0 ? `+${row.score_difference}` : row.score_difference}</td>
              <td className="px-3 py-2 text-center font-semibold tabular-nums">{row.points}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}
