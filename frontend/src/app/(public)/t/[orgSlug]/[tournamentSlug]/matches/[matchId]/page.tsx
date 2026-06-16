import Link from "next/link"
import { notFound } from "next/navigation"
import { ArrowLeftIcon } from "lucide-react"
import { PublicMatchDetail } from "@/components/public/public-match-detail"
import { publicApi, PublicNotFoundError } from "@/lib/api/public"
import type { PublicMatchDetail as MatchDetail } from "@/types/api/public"

interface RouteParams {
  params: Promise<{ orgSlug: string; tournamentSlug: string; matchId: string }>
}

export default async function PublicMatchPage({ params }: RouteParams) {
  const { orgSlug, tournamentSlug, matchId } = await params
  let match: MatchDetail
  try {
    match = await publicApi.match(orgSlug, tournamentSlug, matchId)
  } catch (err) {
    if (err instanceof PublicNotFoundError) notFound()
    throw err
  }
  return (
    <div className="space-y-4">
      <Link
        href={`/t/${orgSlug}/${tournamentSlug}/fixtures`}
        className="inline-flex items-center gap-1 rounded text-sm text-muted-foreground hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
      >
        <ArrowLeftIcon className="size-3.5" />
        All fixtures
      </Link>
      <PublicMatchDetail match={match} />
    </div>
  )
}
