import { notFound } from "next/navigation"
import { PublicBracket } from "@/components/public/public-bracket"
import { publicApi, PublicNotFoundError } from "@/lib/api/public"
import type { PublicMatchesResponse } from "@/types/api/public"

interface RouteParams {
  params: Promise<{ orgSlug: string; tournamentSlug: string }>
}

export default async function PublicBracketPage({ params }: RouteParams) {
  const { orgSlug, tournamentSlug } = await params
  let data: PublicMatchesResponse
  try {
    data = await publicApi.matches(orgSlug, tournamentSlug)
  } catch (err) {
    if (err instanceof PublicNotFoundError) notFound()
    throw err
  }
  return <PublicBracket matches={data.matches} basePath={`/t/${orgSlug}/${tournamentSlug}`} />
}
