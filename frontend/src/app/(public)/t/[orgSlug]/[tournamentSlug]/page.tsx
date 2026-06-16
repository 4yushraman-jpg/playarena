import { notFound } from "next/navigation"
import { CalendarIcon, MapPinIcon, TrophyIcon, CoinsIcon } from "lucide-react"
import { publicApi, PublicNotFoundError } from "@/lib/api/public"
import { formatDate, formatPrizePool } from "@/lib/format"
import type { PublicTournament } from "@/types/api/public"

interface RouteParams {
  params: Promise<{ orgSlug: string; tournamentSlug: string }>
}

export default async function PublicOverviewPage({ params }: RouteParams) {
  const { orgSlug, tournamentSlug } = await params

  let t: PublicTournament
  try {
    t = await publicApi.tournament(orgSlug, tournamentSlug)
  } catch (err) {
    if (err instanceof PublicNotFoundError) notFound()
    throw err
  }

  // Champion banner for a completed tournament (format-agnostic: standings #1).
  let champion: string | null = null
  if (t.status === "completed") {
    try {
      const s = await publicApi.standings(orgSlug, tournamentSlug)
      const top = s.standings.find((r) => !r.disqualified)
      champion = top?.participant_name ?? null
    } catch {
      champion = null
    }
  }

  const details: { icon: React.ReactNode; label: string; value: string }[] = []
  if (t.starts_at) details.push({ icon: <CalendarIcon className="size-4" />, label: "Starts", value: formatDate(t.starts_at) })
  if (t.ends_at) details.push({ icon: <CalendarIcon className="size-4" />, label: "Ends", value: formatDate(t.ends_at) })
  if (t.venue || t.city) {
    details.push({ icon: <MapPinIcon className="size-4" />, label: "Venue", value: [t.venue, t.city].filter(Boolean).join(", ") })
  }
  if (t.prize_pool) {
    details.push({ icon: <CoinsIcon className="size-4" />, label: "Prize pool", value: formatPrizePool(t.prize_pool) })
  }

  return (
    <div className="space-y-6">
      {champion && (
        <div className="flex items-center gap-3 rounded-lg border border-amber-200 bg-amber-50 p-4 dark:border-amber-800 dark:bg-amber-950/30">
          <TrophyIcon className="size-6 text-amber-600 dark:text-amber-400" aria-hidden="true" />
          <div>
            <p className="text-xs uppercase tracking-wide text-amber-700 dark:text-amber-400">Champion</p>
            <p className="text-lg font-bold">{champion}</p>
          </div>
        </div>
      )}

      <p className="text-sm text-muted-foreground">
        {capitalize(t.sport)} · {formatLabel(t.format)}
      </p>

      {t.description && <p className="whitespace-pre-wrap text-sm leading-relaxed">{t.description}</p>}

      {details.length > 0 && (
        <dl className="grid gap-3 sm:grid-cols-2">
          {details.map((d) => (
            <div key={d.label} className="flex flex-col gap-0.5">
              <dt className="flex items-center gap-1.5 text-xs text-muted-foreground">{d.icon}{d.label}</dt>
              <dd className="text-sm font-medium">{d.value}</dd>
            </div>
          ))}
        </dl>
      )}

      {t.rules && (
        <section>
          <h2 className="mb-1 text-sm font-semibold">Rules</h2>
          <p className="whitespace-pre-wrap text-sm text-muted-foreground leading-relaxed">{t.rules}</p>
        </section>
      )}
    </div>
  )
}

function formatLabel(f: string): string {
  return f.replace(/_/g, " ").replace(/\b\w/g, (c) => c.toUpperCase())
}
function capitalize(s: string): string {
  return s.charAt(0).toUpperCase() + s.slice(1)
}
