import { notFound } from "next/navigation"
import { PublicStandings } from "@/components/public/public-standings"
import { publicApi, PublicNotFoundError } from "@/lib/api/public"
import type { PublicStandingsResponse } from "@/types/api/public"

interface RouteParams {
  params: Promise<{ orgSlug: string; tournamentSlug: string }>
}

export default async function PublicStandingsPage({ params }: RouteParams) {
  const { orgSlug, tournamentSlug } = await params
  let data: PublicStandingsResponse
  try {
    data = await publicApi.standings(orgSlug, tournamentSlug)
  } catch (err) {
    if (err instanceof PublicNotFoundError) notFound()
    throw err
  }
  return <PublicStandings data={data} />
}
