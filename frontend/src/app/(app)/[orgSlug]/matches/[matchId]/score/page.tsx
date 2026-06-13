"use client"

import { useParams } from "next/navigation"
import { LiveScorer } from "@/components/scorer/live-scorer"

// Full-bleed live scorer. It lives under (app)/[orgSlug] to inherit the auth
// guard, but LiveScorer renders a fixed full-viewport overlay that covers the
// org shell (sidebar/header) — so the FE-7A layout is untouched and the scorer
// owns the entire screen. FE-7BA is read-only: no scoring controls, no writes.
export default function ScorePage() {
  const params = useParams<{ orgSlug: string; matchId: string }>()
  return <LiveScorer orgSlug={params.orgSlug} matchId={params.matchId} />
}
