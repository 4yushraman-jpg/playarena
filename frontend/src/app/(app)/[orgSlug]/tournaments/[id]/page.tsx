"use client"

import { useState } from "react"
import Link from "next/link"
import { useParams } from "next/navigation"
import {
  TrophyIcon,
  UsersIcon,
  CalendarIcon,
  InfoIcon,
  PlusIcon,
  ListOrderedIcon,
} from "lucide-react"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { StatusBadge } from "@/components/ui/status-badge"
import { Separator } from "@/components/ui/separator"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { PageHeader } from "@/components/ui/page-header"
import { TournamentActions } from "@/components/tournaments/tournament-actions"
import { TournamentTimeline } from "@/components/tournaments/tournament-timeline"
import { RegisterParticipantDialog } from "@/components/tournaments/register-participant-dialog"
import { useTournament, useTournamentStandings } from "@/hooks/use-tournaments"
import { useAuthStore, selectRole } from "@/stores/auth.store"
import { hasPermission } from "@/lib/permissions"
import { getCapacityUsage, getRegistrationCounts } from "@/lib/registration-stats"
import { formatDate, formatDateTime, formatPrizePool, formatRelative } from "@/lib/format"
import { cn } from "@/lib/utils"
import type { Tournament } from "@/types/api/tournaments"

// ── Page ──────────────────────────────────────────────────────────────────────

export default function TournamentDetailPage() {
  const params = useParams<{ orgSlug: string; id: string }>()
  const { orgSlug, id } = params
  const role = useAuthStore(selectRole)
  const canUpdate = hasPermission(role, "tournament.update")
  const canDelete = hasPermission(role, "tournament.delete")

  const [registerOpen, setRegisterOpen] = useState(false)

  const { data: tournament, isLoading, isError } = useTournament(orgSlug, id)

  if (isLoading) {
    return <TournamentDetailSkeleton />
  }

  if (isError || !tournament) {
    return (
      <div className="flex flex-col items-center gap-4 rounded-xl border border-dashed border-border py-24 text-center">
        <TrophyIcon className="size-10 text-muted-foreground/40" />
        <div className="space-y-1">
          <p className="text-sm font-medium">Tournament not found</p>
          <p className="text-xs text-muted-foreground">
            This tournament may have been removed or you may not have access.
          </p>
        </div>
        <Button asChild variant="outline" size="sm">
          <Link href={`/${orgSlug}/tournaments`}>Back to Tournaments</Link>
        </Button>
      </div>
    )
  }

  const counts = getRegistrationCounts(tournament)
  const isRegistrationOpen = tournament.status === "registration_open"
  const showStandings =
    tournament.status === "ongoing" || tournament.status === "completed"
  const participantNoun =
    tournament.participant_type === "team" ? "team" : "player"

  return (
    <div className="space-y-6">
      <PageHeader
        title={tournament.name}
        breadcrumbs={[
          { label: "Dashboard", href: `/${orgSlug}` },
          { label: "Tournaments", href: `/${orgSlug}/tournaments` },
          { label: tournament.name },
        ]}
        action={
          canUpdate && isRegistrationOpen ? (
            <Button size="sm" className="gap-2" onClick={() => setRegisterOpen(true)}>
              <PlusIcon className="size-3.5" />
              Register {participantNoun}
            </Button>
          ) : undefined
        }
      />

      {/* Status + sport banner */}
      <div className="flex flex-wrap items-center gap-3">
        <StatusBadge status={tournament.status} />
        <span className="text-sm text-muted-foreground capitalize">
          {tournament.sport}
        </span>
        <span className="text-muted-foreground/40" aria-hidden="true">·</span>
        <span className="text-sm text-muted-foreground capitalize">
          {tournament.format.replace(/_/g, " ")}
        </span>
        <span className="text-muted-foreground/40" aria-hidden="true">·</span>
        <span className="text-sm text-muted-foreground capitalize">
          {tournament.participant_type} tournament
        </span>
      </div>

      {/* Lifecycle timeline */}
      <div className="rounded-xl border border-border bg-card p-4">
        <TournamentTimeline status={tournament.status} />
      </div>

      {/* Actions */}
      <TournamentActions
        tournament={tournament}
        orgSlug={orgSlug}
        canUpdate={canUpdate}
        canDelete={canDelete}
      />

      {/* Main grid */}
      <div className="grid gap-6 lg:grid-cols-3">
        {/* Left: Details (2/3) */}
        <div className="space-y-6 lg:col-span-2">
          {/* Overview */}
          <Card>
            <CardHeader className="pb-3">
              <CardTitle className="flex items-center gap-2 text-base">
                <InfoIcon className="size-4 text-muted-foreground" />
                Overview
              </CardTitle>
            </CardHeader>
            <CardContent className="space-y-4">
              {tournament.description && (
                <p className="text-sm text-muted-foreground leading-relaxed">
                  {tournament.description}
                </p>
              )}
              <dl className="grid gap-3 sm:grid-cols-2">
                <DetailRow label="Created" value={formatRelative(tournament.created_at)} />
                <DetailRow label="Starts" value={formatDate(tournament.starts_at)} />
                <DetailRow label="Ends" value={formatDate(tournament.ends_at)} />
                {tournament.venue && (
                  <DetailRow label="Venue" value={tournament.venue} />
                )}
                {tournament.city && (
                  <DetailRow
                    label="Location"
                    value={[tournament.city, tournament.country].filter(Boolean).join(", ")}
                  />
                )}
                {tournament.prize_pool && (
                  <DetailRow
                    label="Prize pool"
                    value={formatPrizePool(tournament.prize_pool)}
                  />
                )}
              </dl>
            </CardContent>
          </Card>

          {/* Standings — live for ongoing, final for completed */}
          {showStandings && (
            <StandingsCard orgSlug={orgSlug} tournamentId={id} tournament={tournament} />
          )}

          {/* Registration window */}
          {(tournament.registration_opens_at || tournament.registration_closes_at) && (
            <Card>
              <CardHeader className="pb-3">
                <CardTitle className="flex items-center gap-2 text-base">
                  <CalendarIcon className="size-4 text-muted-foreground" />
                  Registration window
                </CardTitle>
              </CardHeader>
              <CardContent>
                <dl className="grid gap-3 sm:grid-cols-2">
                  {tournament.registration_opens_at && (
                    <DetailRow
                      label="Opens"
                      value={formatDateTime(tournament.registration_opens_at)}
                    />
                  )}
                  {tournament.registration_closes_at && (
                    <DetailRow
                      label="Closes"
                      value={formatDateTime(tournament.registration_closes_at)}
                    />
                  )}
                </dl>
              </CardContent>
            </Card>
          )}

          {/* Rules */}
          {tournament.rules && (
            <Card>
              <CardHeader className="pb-3">
                <CardTitle className="text-base">Rules</CardTitle>
              </CardHeader>
              <CardContent>
                <p className="whitespace-pre-wrap text-sm text-muted-foreground leading-relaxed">
                  {tournament.rules}
                </p>
              </CardContent>
            </Card>
          )}
        </div>

        {/* Right: Health (1/3) */}
        <div className="space-y-4">
          {/* Registration health — server-aggregated counts, exact at any scale */}
          <Card>
            <CardHeader className="pb-3">
              <CardTitle className="flex items-center gap-2 text-base">
                <UsersIcon className="size-4 text-muted-foreground" />
                Registration health
              </CardTitle>
            </CardHeader>
            <CardContent className="space-y-4">
              <CapacityBar tournament={tournament} />

              <Separator />

              {/* Status breakdown */}
              <dl className="space-y-2">
                <HealthRow
                  label="Approved"
                  count={counts.approved}
                  colorClass="text-green-600 dark:text-green-400"
                  dotClass="bg-green-500"
                />
                <HealthRow
                  label="Pending"
                  count={counts.pending}
                  colorClass="text-amber-600 dark:text-amber-400"
                  dotClass="bg-amber-500"
                />
                <HealthRow
                  label="Rejected"
                  count={counts.rejected}
                  colorClass="text-red-600 dark:text-red-400"
                  dotClass="bg-red-400"
                />
                <HealthRow
                  label="Withdrawn"
                  count={counts.withdrawn}
                  colorClass="text-muted-foreground"
                  dotClass="bg-muted-foreground/40"
                />
                {counts.disqualified > 0 && (
                  <HealthRow
                    label="Disqualified"
                    count={counts.disqualified}
                    colorClass="text-red-700 dark:text-red-300"
                    dotClass="bg-red-600"
                  />
                )}
              </dl>

              {/* Quick link */}
              {canUpdate && (
                <>
                  <Separator />
                  <Link
                    href={`/${orgSlug}/tournaments/${id}/registrations`}
                    className="flex items-center justify-center gap-1.5 rounded-md border border-border px-3 py-2 text-xs font-medium transition-colors hover:bg-accent"
                  >
                    <UsersIcon className="size-3" />
                    Manage registrations
                    {counts.pending > 0 && (
                      <span
                        className="ml-auto flex h-4 min-w-4 items-center justify-center rounded-full bg-amber-500 px-1 text-[10px] font-bold text-white"
                        aria-label={`${counts.pending} pending`}
                      >
                        {counts.pending}
                      </span>
                    )}
                  </Link>
                </>
              )}
            </CardContent>
          </Card>

          {/* Quick stats */}
          <Card>
            <CardHeader className="pb-3">
              <CardTitle className="text-base">Details</CardTitle>
            </CardHeader>
            <CardContent>
              <dl className="space-y-2 text-sm">
                <div className="flex items-center justify-between">
                  <dt className="text-muted-foreground">Format</dt>
                  <dd className="font-medium capitalize">{tournament.format.replace(/_/g, " ")}</dd>
                </div>
                <div className="flex items-center justify-between">
                  <dt className="text-muted-foreground">Participant</dt>
                  <dd className="font-medium capitalize">{tournament.participant_type}</dd>
                </div>
                {tournament.min_participants && (
                  <div className="flex items-center justify-between">
                    <dt className="text-muted-foreground">Min participants</dt>
                    <dd className="font-medium tabular-nums">{tournament.min_participants}</dd>
                  </div>
                )}
                {tournament.max_participants && (
                  <div className="flex items-center justify-between">
                    <dt className="text-muted-foreground">Max participants</dt>
                    <dd className="font-medium tabular-nums">{tournament.max_participants}</dd>
                  </div>
                )}
                <div className="flex items-center justify-between">
                  <dt className="text-muted-foreground">Currency</dt>
                  <dd className="font-medium">{tournament.currency}</dd>
                </div>
              </dl>
            </CardContent>
          </Card>
        </div>
      </div>

      {/* Register participant dialog */}
      {canUpdate && (
        <RegisterParticipantDialog
          open={registerOpen}
          onOpenChange={setRegisterOpen}
          tournament={tournament}
          orgSlug={orgSlug}
        />
      )}
    </div>
  )
}

// ── Capacity bar ──────────────────────────────────────────────────────────────

function CapacityBar({ tournament }: { tournament: Tournament }) {
  const usage = getCapacityUsage(tournament)
  const counts = getRegistrationCounts(tournament)

  if (!usage) {
    return (
      <div className="flex items-center justify-between text-xs">
        <span className="text-muted-foreground">Active registrations</span>
        <span className="tabular-nums font-medium">{counts.active}</span>
      </div>
    )
  }

  return (
    <div className="space-y-1.5">
      <div className="flex items-center justify-between text-xs">
        <span className="text-muted-foreground">Capacity</span>
        <span className="tabular-nums font-medium">
          {usage.used} / {usage.max}
        </span>
      </div>
      <div
        className="h-1.5 w-full overflow-hidden rounded-full bg-muted"
        role="progressbar"
        aria-valuemin={0}
        aria-valuenow={usage.used}
        aria-valuemax={usage.max}
        aria-valuetext={`${usage.used} of ${usage.max} spots used (pending and approved)`}
        aria-label="Registration capacity"
      >
        <div
          className={cn(
            "h-full rounded-full transition-all",
            usage.pct >= 90
              ? "bg-red-500"
              : usage.pct >= 70
              ? "bg-amber-500"
              : "bg-primary",
          )}
          style={{ width: `${usage.pct}%` }}
        />
      </div>
      <p className="text-[11px] text-muted-foreground">
        Counts pending + approved — the limit the server enforces.
      </p>
    </div>
  )
}

// ── Standings ─────────────────────────────────────────────────────────────────

function StandingsCard({
  orgSlug,
  tournamentId,
  tournament,
}: {
  orgSlug: string
  tournamentId: string
  tournament: Tournament
}) {
  const { data, isLoading, isError, refetch } = useTournamentStandings(orgSlug, tournamentId)
  const rows = data?.standings ?? []

  return (
    <Card>
      <CardHeader className="pb-3">
        <CardTitle className="flex items-center gap-2 text-base">
          <ListOrderedIcon className="size-4 text-muted-foreground" />
          {tournament.status === "completed" ? "Final standings" : "Standings"}
        </CardTitle>
      </CardHeader>
      <CardContent>
        {isLoading ? (
          <div className="space-y-2" aria-busy="true" aria-label="Loading standings">
            {Array.from({ length: 4 }).map((_, i) => (
              <Skeleton key={i} className="h-8 w-full" />
            ))}
          </div>
        ) : isError ? (
          <div className="flex flex-col items-start gap-2">
            <p className="text-sm text-muted-foreground">Failed to load standings.</p>
            <Button variant="outline" size="sm" onClick={() => refetch()}>
              Retry
            </Button>
          </div>
        ) : rows.length === 0 ? (
          <p className="text-sm text-muted-foreground">
            Standings will appear once participants are approved and matches complete.
          </p>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm" aria-label="Tournament standings">
              <thead>
                <tr className="border-b border-border text-left text-xs font-medium text-muted-foreground">
                  <th scope="col" className="py-2 pr-2">#</th>
                  <th scope="col" className="py-2 pr-2">Participant</th>
                  <th scope="col" className="py-2 pr-2 text-right">P</th>
                  <th scope="col" className="hidden py-2 pr-2 text-right sm:table-cell">W</th>
                  <th scope="col" className="hidden py-2 pr-2 text-right sm:table-cell">D</th>
                  <th scope="col" className="hidden py-2 pr-2 text-right sm:table-cell">L</th>
                  <th scope="col" className="hidden py-2 pr-2 text-right md:table-cell">+/−</th>
                  <th scope="col" className="py-2 text-right">Pts</th>
                </tr>
              </thead>
              <tbody>
                {rows.map((row) => (
                  <tr key={row.participant_id} className="border-b border-border/50 last:border-0">
                    <td className="py-2 pr-2 tabular-nums text-muted-foreground">
                      {row.position}
                    </td>
                    <td className="max-w-44 truncate py-2 pr-2 font-medium">
                      {row.participant_name || "Unknown participant"}
                    </td>
                    <td className="py-2 pr-2 text-right tabular-nums">{row.played}</td>
                    <td className="hidden py-2 pr-2 text-right tabular-nums sm:table-cell">{row.wins}</td>
                    <td className="hidden py-2 pr-2 text-right tabular-nums sm:table-cell">{row.draws}</td>
                    <td className="hidden py-2 pr-2 text-right tabular-nums sm:table-cell">{row.losses}</td>
                    <td className="hidden py-2 pr-2 text-right tabular-nums md:table-cell">
                      {row.score_difference > 0 ? `+${row.score_difference}` : row.score_difference}
                    </td>
                    <td className="py-2 text-right font-semibold tabular-nums">{row.points}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </CardContent>
    </Card>
  )
}

// ── Sub-components ────────────────────────────────────────────────────────────

function DetailRow({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex flex-col gap-0.5">
      <dt className="text-xs text-muted-foreground">{label}</dt>
      <dd className="text-sm font-medium">{value}</dd>
    </div>
  )
}

function HealthRow({
  label,
  count,
  colorClass,
  dotClass,
}: {
  label: string
  count: number
  colorClass: string
  dotClass: string
}) {
  return (
    <div className="flex items-center justify-between">
      <dt className="flex items-center gap-1.5 text-xs text-muted-foreground">
        <span className={cn("size-1.5 rounded-full", dotClass)} aria-hidden="true" />
        {label}
      </dt>
      <dd className={cn("tabular-nums text-sm font-semibold", colorClass)}>{count}</dd>
    </div>
  )
}

// ── Loading skeleton ──────────────────────────────────────────────────────────

function TournamentDetailSkeleton() {
  return (
    <div className="space-y-6" aria-busy="true" aria-label="Loading tournament">
      <div className="space-y-2">
        <Skeleton className="h-4 w-64" />
        <Skeleton className="h-8 w-48" />
      </div>
      <Skeleton className="h-5 w-32 rounded-full" />
      <div className="rounded-xl border border-border bg-card p-4">
        <Skeleton className="h-10 w-full" />
      </div>
      <div className="grid gap-6 lg:grid-cols-3">
        <div className="space-y-4 lg:col-span-2">
          <div className="rounded-xl border border-border p-6 space-y-4">
            <Skeleton className="h-5 w-24" />
            {Array.from({ length: 4 }).map((_, i) => (
              <Skeleton key={i} className="h-4 w-full" />
            ))}
          </div>
        </div>
        <div className="space-y-4">
          <div className="rounded-xl border border-border p-6 space-y-3">
            <Skeleton className="h-5 w-32" />
            {Array.from({ length: 4 }).map((_, i) => (
              <Skeleton key={i} className="h-8 w-full" />
            ))}
          </div>
        </div>
      </div>
    </div>
  )
}
