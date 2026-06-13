"use client"

import Link from "next/link"
import { useParams } from "next/navigation"
import { SwordsIcon, CalendarIcon, MapPinIcon, TrophyIcon, InfoIcon } from "lucide-react"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { StatusBadge } from "@/components/ui/status-badge"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { PageHeader } from "@/components/ui/page-header"
import { MatchActions } from "@/components/matches/match-actions"
import { useMatch } from "@/hooks/use-matches"
import { useTournament } from "@/hooks/use-tournaments"
import { useParticipantNames } from "@/hooks/use-participant-names"
import { useAuthStore, selectRole } from "@/stores/auth.store"
import { hasPermission } from "@/lib/permissions"
import { formatMatchLabel, matchParticipantIds, matchParticipantType } from "@/lib/match-meta"
import { formatDateTime } from "@/lib/format"
import { cn } from "@/lib/utils"

export default function MatchDetailPage() {
  const params = useParams<{ orgSlug: string; matchId: string }>()
  const { orgSlug, matchId } = params
  const role = useAuthStore(selectRole)
  const canUpdate = hasPermission(role, "match.update")
  const canDelete = hasPermission(role, "match.delete")

  const { data: match, isLoading, isError } = useMatch(orgSlug, matchId)
  const { resolve } = useParticipantNames(orgSlug)
  // Tournament context (name + link). Enabled once we know the match.
  const { data: tournament } = useTournament(orgSlug, match?.tournament_id ?? "")

  if (isLoading) {
    return <MatchDetailSkeleton />
  }

  if (isError || !match) {
    return (
      <div className="flex flex-col items-center gap-4 rounded-xl border border-dashed border-border py-24 text-center">
        <SwordsIcon className="size-10 text-muted-foreground/40" />
        <div className="space-y-1">
          <p className="text-sm font-medium">Match not found</p>
          <p className="text-xs text-muted-foreground">
            This match may have been removed or you may not have access.
          </p>
        </div>
        <Button asChild variant="outline" size="sm">
          <Link href={`/${orgSlug}/matches`}>Back to Matches</Link>
        </Button>
      </div>
    )
  }

  const isTeam = matchParticipantType(match) === "team"
  const { homeId, awayId, winnerId } = matchParticipantIds(match)
  const homeName = resolve(isTeam ? homeId : null, isTeam ? null : homeId)
  const awayName = resolve(isTeam ? awayId : null, isTeam ? null : awayId)
  const isCompleted = match.status === "completed"
  const isWalkover = match.status === "walkover"
  // A walkover concludes the match with a winner but no meaningful score.
  const isConcluded = isCompleted || isWalkover
  const homeWon = isConcluded && winnerId != null && winnerId === homeId
  const awayWon = isConcluded && winnerId != null && winnerId === awayId
  const isDraw = isCompleted && !match.is_walkover && winnerId == null

  return (
    <div className="space-y-6">
      <PageHeader
        title={`${homeName} vs ${awayName}`}
        breadcrumbs={[
          { label: "Dashboard", href: `/${orgSlug}` },
          { label: "Matches", href: `/${orgSlug}/matches` },
          { label: `${homeName} vs ${awayName}` },
        ]}
      />

      {/* Status + context banner */}
      <div className="flex flex-wrap items-center gap-3">
        <StatusBadge status={match.status} />
        <span className="text-sm text-muted-foreground">{formatMatchLabel(match)}</span>
        {tournament && (
          <>
            <span className="text-muted-foreground/40" aria-hidden="true">·</span>
            <Link
              href={`/${orgSlug}/tournaments/${match.tournament_id}`}
              className="text-sm text-muted-foreground hover:text-foreground hover:underline"
            >
              {tournament.name}
            </Link>
          </>
        )}
      </div>

      {/* Actions (edit/cancel for scheduled; walkover for scheduled or live) */}
      <MatchActions
        match={match}
        orgSlug={orgSlug}
        canUpdate={canUpdate}
        canDelete={canDelete}
        homeName={homeName}
        awayName={awayName}
      />

      {/* Scoreboard */}
      <Card>
        <CardContent className="py-6">
          <div className="grid grid-cols-[1fr_auto_1fr] items-center gap-4">
            <ParticipantSide name={homeName} won={homeWon} align="right" />
            <div className="flex flex-col items-center gap-1">
              {isCompleted ? (
                <span className="tabular-nums text-3xl font-bold">
                  {match.home_score} <span className="text-muted-foreground">–</span>{" "}
                  {match.away_score}
                </span>
              ) : isWalkover ? (
                <span className="text-2xl font-bold uppercase tracking-wide text-amber-600 dark:text-amber-400">
                  W/O
                </span>
              ) : (
                <span className="text-sm font-medium uppercase tracking-wide text-muted-foreground">
                  vs
                </span>
              )}
              {isDraw && <span className="text-xs text-muted-foreground">Draw</span>}
              {match.is_walkover && (
                <span className="text-xs font-medium text-amber-600 dark:text-amber-400">
                  Walkover
                </span>
              )}
            </div>
            <ParticipantSide name={awayName} won={awayWon} align="left" />
          </div>
        </CardContent>
      </Card>

      {/* Details */}
      <Card>
        <CardHeader className="pb-3">
          <CardTitle className="flex items-center gap-2 text-base">
            <InfoIcon className="size-4 text-muted-foreground" />
            Details
          </CardTitle>
        </CardHeader>
        <CardContent>
          <dl className="grid gap-3 sm:grid-cols-2">
            <DetailRow
              icon={<CalendarIcon className="size-3.5" />}
              label="Scheduled"
              value={match.scheduled_at ? formatDateTime(match.scheduled_at) : "—"}
            />
            {match.started_at && (
              <DetailRow
                icon={<CalendarIcon className="size-3.5" />}
                label="Started"
                value={formatDateTime(match.started_at)}
              />
            )}
            {match.ended_at && (
              <DetailRow
                icon={<CalendarIcon className="size-3.5" />}
                label="Ended"
                value={formatDateTime(match.ended_at)}
              />
            )}
            <DetailRow
              icon={<MapPinIcon className="size-3.5" />}
              label="Venue"
              value={match.venue || "—"}
            />
            {isConcluded && !isDraw && (winnerId === homeId || winnerId === awayId) && (
              <DetailRow
                icon={<TrophyIcon className="size-3.5" />}
                label={isWalkover ? "Winner (walkover)" : "Winner"}
                value={winnerId === homeId ? homeName : awayName}
              />
            )}
          </dl>
          {match.notes && (
            <p className="mt-4 whitespace-pre-wrap text-sm text-muted-foreground leading-relaxed">
              {match.notes}
            </p>
          )}
        </CardContent>
      </Card>
    </div>
  )
}

// ── Sub-components ────────────────────────────────────────────────────────────

function ParticipantSide({
  name,
  won,
  align,
}: {
  name: string
  won: boolean
  align: "left" | "right"
}) {
  return (
    <div className={cn("min-w-0", align === "right" ? "text-right" : "text-left")}>
      <p className={cn("truncate text-base font-semibold", won && "text-primary")}>
        {name}
      </p>
      {won && (
        <span className="mt-0.5 inline-flex items-center gap-1 text-xs font-medium text-primary">
          <TrophyIcon className="size-3" />
          Winner
        </span>
      )}
    </div>
  )
}

function DetailRow({
  icon,
  label,
  value,
}: {
  icon: React.ReactNode
  label: string
  value: string
}) {
  return (
    <div className="flex flex-col gap-0.5">
      <dt className="flex items-center gap-1.5 text-xs text-muted-foreground">
        {icon}
        {label}
      </dt>
      <dd className="text-sm font-medium">{value}</dd>
    </div>
  )
}

function MatchDetailSkeleton() {
  return (
    <div className="space-y-6" aria-busy="true" aria-label="Loading match">
      <div className="space-y-2">
        <Skeleton className="h-4 w-64" />
        <Skeleton className="h-8 w-72" />
      </div>
      <Skeleton className="h-5 w-40 rounded-full" />
      <Skeleton className="h-28 w-full rounded-xl" />
      <Skeleton className="h-40 w-full rounded-xl" />
    </div>
  )
}
