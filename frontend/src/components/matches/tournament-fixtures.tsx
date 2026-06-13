"use client"

import { useState } from "react"
import Link from "next/link"
import { SwordsIcon, PlusIcon } from "lucide-react"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { StatusBadge } from "@/components/ui/status-badge"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { CreateFixtureDialog } from "./create-fixture-dialog"
import { useMatchList } from "@/hooks/use-matches"
import { useParticipantNames } from "@/hooks/use-participant-names"
import { formatMatchLabel, matchParticipantIds } from "@/lib/match-meta"
import { formatDateTime, formatScore } from "@/lib/format"
import type { Tournament } from "@/types/api/tournaments"

const FIXTURE_LIMIT = 100

interface TournamentFixturesProps {
  orgSlug: string
  tournament: Tournament
  canCreate: boolean
}

export function TournamentFixtures({
  orgSlug,
  tournament,
  canCreate,
}: TournamentFixturesProps) {
  const [createOpen, setCreateOpen] = useState(false)

  const { data, isLoading, isError, refetch } = useMatchList(orgSlug, {
    tournament_id: tournament.id,
    limit: FIXTURE_LIMIT,
  })
  const { resolve } = useParticipantNames(orgSlug)

  const matches = data?.matches ?? []
  // Matches can only be created while the tournament is ongoing — the backend
  // rejects creation otherwise (ErrTournamentNotOngoing).
  const canAddFixture = canCreate && tournament.status === "ongoing"

  return (
    <Card>
      <CardHeader className="flex flex-row items-center justify-between gap-2 pb-3">
        <CardTitle className="flex items-center gap-2 text-base">
          <SwordsIcon className="size-4 text-muted-foreground" />
          Fixtures
        </CardTitle>
        {canAddFixture && (
          <Button size="sm" className="gap-1.5" onClick={() => setCreateOpen(true)}>
            <PlusIcon className="size-3.5" />
            Create fixture
          </Button>
        )}
      </CardHeader>
      <CardContent>
        {isLoading ? (
          <div className="space-y-2" aria-busy="true" aria-label="Loading fixtures">
            {Array.from({ length: 3 }).map((_, i) => (
              <Skeleton key={i} className="h-10 w-full" />
            ))}
          </div>
        ) : isError ? (
          <div className="flex flex-col items-start gap-2">
            <p className="text-sm text-muted-foreground">Failed to load fixtures.</p>
            <Button variant="outline" size="sm" onClick={() => refetch()}>
              Retry
            </Button>
          </div>
        ) : matches.length === 0 ? (
          <p className="text-sm text-muted-foreground">
            {canAddFixture
              ? "No fixtures yet. Create your first fixture to schedule a match."
              : "No fixtures have been scheduled for this tournament yet."}
          </p>
        ) : (
          <ul className="divide-y divide-border">
            {matches.map((m) => {
              const { homeId, awayId } = matchParticipantIds(m)
              const isTeam = !!(m.home_team_id || m.away_team_id)
              const homeName = resolve(isTeam ? homeId : null, isTeam ? null : homeId)
              const awayName = resolve(isTeam ? awayId : null, isTeam ? null : awayId)
              const showScore = m.status === "completed"
              return (
                <li key={m.id}>
                  <Link
                    href={`/${orgSlug}/matches/${m.id}`}
                    className="flex items-center gap-3 py-2.5 transition-colors hover:bg-accent/40 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:rounded"
                  >
                    <div className="min-w-0 flex-1">
                      <p className="truncate text-sm font-medium">
                        {homeName} <span className="text-muted-foreground">vs</span> {awayName}
                      </p>
                      <p className="mt-0.5 truncate text-xs text-muted-foreground">
                        {formatMatchLabel(m)}
                        {m.scheduled_at ? ` · ${formatDateTime(m.scheduled_at)}` : ""}
                      </p>
                    </div>
                    {showScore && (
                      <span className="shrink-0 tabular-nums text-sm font-semibold">
                        {formatScore(m.home_score, m.away_score)}
                      </span>
                    )}
                    <StatusBadge status={m.status} />
                  </Link>
                </li>
              )
            })}
          </ul>
        )}
      </CardContent>

      {canAddFixture && (
        <CreateFixtureDialog
          open={createOpen}
          onOpenChange={setCreateOpen}
          tournament={tournament}
          orgSlug={orgSlug}
        />
      )}
    </Card>
  )
}
