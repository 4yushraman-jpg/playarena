import { notFound } from "next/navigation"
import { PublicFixtures } from "@/components/public/public-fixtures"
import { publicApi, PublicNotFoundError } from "@/lib/api/public"
import type { PublicMatchesResponse } from "@/types/api/public"

interface RouteParams {
  params: Promise<{ orgSlug: string; tournamentSlug: string }>
}

export default async function PublicResultsPage({ params }: RouteParams) {
  const { orgSlug, tournamentSlug } = await params
  let data: PublicMatchesResponse
  try {
    data = await publicApi.matches(orgSlug, tournamentSlug)
  } catch (err) {
    if (err instanceof PublicNotFoundError) notFound()
    throw err
  }
  return (
    <PublicFixtures
      matches={data.matches}
      basePath={`/t/${orgSlug}/${tournamentSlug}`}
      resultsOnly
      emptyLabel="No results yet — check back once matches are played."
    />
  )
}
