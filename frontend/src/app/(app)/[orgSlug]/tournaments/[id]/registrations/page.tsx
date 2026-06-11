"use client"

import { useId, useState } from "react"
import { useParams } from "next/navigation"
import {
  CheckIcon,
  XIcon,
  MinusCircleIcon,
  UsersIcon,
  PlusIcon,
  Loader2Icon,
  MoreHorizontalIcon,
} from "lucide-react"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { StatusBadge } from "@/components/ui/status-badge"
import { EmptyState } from "@/components/ui/empty-state"
import { PageHeader } from "@/components/ui/page-header"
import { ConfirmDialog } from "@/components/ui/confirm-dialog"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { RegisterParticipantDialog } from "@/components/tournaments/register-participant-dialog"
import { useTournament } from "@/hooks/use-tournaments"
import {
  useRegistrationList,
  useUpdateRegistration,
  useWithdrawRegistration,
} from "@/hooks/use-registrations"
import { useAuthStore, selectRole } from "@/stores/auth.store"
import { hasPermission } from "@/lib/permissions"
import { getParticipantLabel, getRegistrationCounts } from "@/lib/registration-stats"
import { formatRelative } from "@/lib/format"
import { cn } from "@/lib/utils"
import type {
  RegistrationStatus,
  TournamentRegistration,
} from "@/types/api/tournament-registrations"
import type { Tournament, RegistrationCounts } from "@/types/api/tournaments"

// ── Status tab config ──────────────────────────────────────────────────────────

type TabStatus = "all" | RegistrationStatus

interface Tab {
  value: TabStatus
  label: string
}

const TABS: Tab[] = [
  { value: "all", label: "All" },
  { value: "pending", label: "Pending" },
  { value: "approved", label: "Approved" },
  { value: "rejected", label: "Rejected" },
  { value: "withdrawn", label: "Withdrawn" },
  { value: "disqualified", label: "Disqualified" },
]

function tabCount(counts: RegistrationCounts, tab: TabStatus): number {
  return tab === "all" ? counts.total : counts[tab]
}

const PAGE_SIZE = 50

// ── Page ──────────────────────────────────────────────────────────────────────

export default function RegistrationDashboardPage() {
  const params = useParams<{ orgSlug: string; id: string }>()
  const { orgSlug, id } = params
  const role = useAuthStore(selectRole)
  const canUpdate = hasPermission(role, "tournament.update")

  const [activeTab, setActiveTab] = useState<TabStatus>("all")
  const [page, setPage] = useState(0)
  const [registerOpen, setRegisterOpen] = useState(false)
  const statsHeadingId = useId()

  const { data: tournament, isLoading: tournamentLoading } = useTournament(orgSlug, id)

  // Server-side filtering and pagination. Tab counts come from the
  // server-aggregated registration_counts so they stay exact regardless of
  // how many registrations exist.
  const {
    data: registrationData,
    isLoading: regsLoading,
    isError: regsError,
    refetch: refetchRegs,
  } = useRegistrationList(orgSlug, id, {
    limit: PAGE_SIZE,
    offset: page * PAGE_SIZE,
    status: activeTab === "all" ? undefined : activeTab,
  })

  const registrations = registrationData?.registrations ?? []
  const filteredTotal = registrationData?.total ?? 0
  const totalPages = Math.ceil(filteredTotal / PAGE_SIZE)

  const counts = tournament ? getRegistrationCounts(tournament) : null

  function selectTab(tab: TabStatus) {
    setActiveTab(tab)
    setPage(0)
  }

  const isLoading = tournamentLoading || regsLoading
  const isRegistrationOpen = tournament?.status === "registration_open"
  const participantNoun =
    tournament?.participant_type === "individual" ? "player" : "team"

  return (
    <div className="space-y-6">
      <PageHeader
        title="Registrations"
        description={tournament ? `Managing registrations for ${tournament.name}` : undefined}
        breadcrumbs={[
          { label: "Dashboard", href: `/${orgSlug}` },
          { label: "Tournaments", href: `/${orgSlug}/tournaments` },
          { label: tournament?.name ?? "…", href: `/${orgSlug}/tournaments/${id}` },
          { label: "Registrations" },
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

      {/* Summary stats */}
      {counts && (
        <section aria-labelledby={statsHeadingId}>
          <h2 id={statsHeadingId} className="sr-only">
            Registration summary
          </h2>
          <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
            <StatCard label="Total" value={counts.total} />
            <StatCard
              label="Approved"
              value={counts.approved}
              colorClass="text-green-600 dark:text-green-400"
            />
            <StatCard
              label="Pending"
              value={counts.pending}
              colorClass="text-amber-600 dark:text-amber-400"
            />
            <StatCard
              label="Inactive"
              description="Rejected, withdrawn & disqualified"
              value={counts.rejected + counts.withdrawn + counts.disqualified}
              colorClass="text-muted-foreground"
            />
          </div>
        </section>
      )}

      {/* Tabs */}
      <div
        className="flex items-center gap-1 overflow-x-auto rounded-lg border border-border bg-muted/30 p-1"
        role="tablist"
        aria-label="Filter registrations by status"
      >
        {TABS.map((tab) => (
          <button
            key={tab.value}
            role="tab"
            aria-selected={activeTab === tab.value}
            className={cn(
              "flex shrink-0 items-center gap-1.5 rounded-md px-3 py-1.5 text-sm font-medium transition-colors",
              activeTab === tab.value
                ? "bg-background text-foreground shadow-sm"
                : "text-muted-foreground hover:text-foreground",
            )}
            onClick={() => selectTab(tab.value)}
          >
            {tab.label}
            {counts && (
              <span
                className={cn(
                  "min-w-4 rounded-full px-1 text-center text-xs tabular-nums",
                  activeTab === tab.value
                    ? "bg-primary/10 text-primary"
                    : "bg-muted text-muted-foreground",
                )}
              >
                {tabCount(counts, tab.value)}
              </span>
            )}
          </button>
        ))}
      </div>

      {/* Table */}
      {isLoading ? (
        <RegistrationsTableSkeleton />
      ) : regsError ? (
        <EmptyState
          icon={<UsersIcon />}
          title="Failed to load registrations"
          description="Something went wrong fetching registrations. Please try again."
          action={
            <Button variant="outline" size="sm" onClick={() => refetchRegs()}>
              Retry
            </Button>
          }
        />
      ) : registrations.length === 0 ? (
        <EmptyState
          icon={<UsersIcon />}
          title={activeTab === "all" ? "No registrations yet" : `No ${activeTab} registrations`}
          description={
            activeTab === "all"
              ? isRegistrationOpen
                ? `${participantNoun === "team" ? "Teams" : "Players"} can register once you share the tournament.`
                : "Registrations will appear here."
              : `No registrations with ${activeTab} status.`
          }
        />
      ) : tournament ? (
        <>
          <RegistrationsTable
            registrations={registrations}
            tournament={tournament}
            orgSlug={orgSlug}
            canUpdate={canUpdate}
          />
          {totalPages > 1 && (
            <nav
              className="flex items-center justify-between text-sm text-muted-foreground"
              aria-label="Registration pages"
            >
              <span>
                {filteredTotal} registration{filteredTotal !== 1 ? "s" : ""}
              </span>
              <div className="flex items-center gap-2">
                <Button
                  variant="outline"
                  size="sm"
                  className="min-h-9 px-4"
                  disabled={page === 0}
                  onClick={() => setPage((p) => p - 1)}
                >
                  Previous
                </Button>
                <span className="tabular-nums" aria-live="polite">
                  {page + 1} / {totalPages}
                </span>
                <Button
                  variant="outline"
                  size="sm"
                  className="min-h-9 px-4"
                  disabled={page >= totalPages - 1}
                  onClick={() => setPage((p) => p + 1)}
                >
                  Next
                </Button>
              </div>
            </nav>
          )}
        </>
      ) : null}

      {/* Register dialog */}
      {tournament && canUpdate && (
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

// ── Registrations table ───────────────────────────────────────────────────────

interface RegistrationsTableProps {
  registrations: TournamentRegistration[]
  tournament: Tournament
  orgSlug: string
  canUpdate: boolean
}

function RegistrationsTable({
  registrations,
  tournament,
  orgSlug,
  canUpdate,
}: RegistrationsTableProps) {
  return (
    <div className="overflow-hidden rounded-xl border border-border">
      <div className="overflow-x-auto">
        <table className="w-full text-sm" aria-label="Tournament registrations">
          <thead>
            <tr className="bg-muted/40 text-left text-xs font-medium text-muted-foreground">
              <th scope="col" className="px-4 py-2.5">Participant</th>
              <th scope="col" className="px-4 py-2.5">Status</th>
              <th scope="col" className="hidden px-4 py-2.5 sm:table-cell">Registered</th>
              <th scope="col" className="hidden px-4 py-2.5 md:table-cell">Approved</th>
              {canUpdate && (
                <th scope="col" className="px-4 py-2.5 text-right">Actions</th>
              )}
            </tr>
          </thead>
          <tbody>
            {registrations.map((reg) => (
              <RegistrationRow
                key={reg.id}
                registration={reg}
                tournament={tournament}
                orgSlug={orgSlug}
                canUpdate={canUpdate}
              />
            ))}
          </tbody>
        </table>
      </div>
    </div>
  )
}

// ── Registration row ──────────────────────────────────────────────────────────

interface RegistrationRowProps {
  registration: TournamentRegistration
  tournament: Tournament
  orgSlug: string
  canUpdate: boolean
}

function RegistrationRow({
  registration,
  tournament,
  orgSlug,
  canUpdate,
}: RegistrationRowProps) {
  const updateRegistration = useUpdateRegistration(orgSlug, tournament.id)
  const withdrawRegistration = useWithdrawRegistration(orgSlug, tournament.id)

  const [confirmWithdraw, setConfirmWithdraw] = useState(false)
  const [confirmReject, setConfirmReject] = useState(false)

  const isTerminal =
    registration.status === "rejected" ||
    registration.status === "withdrawn" ||
    registration.status === "disqualified"

  const canApprove = canUpdate && registration.status === "pending"
  const canReject = canUpdate && registration.status === "pending"
  const canWithdraw =
    canUpdate &&
    (registration.status === "pending" || registration.status === "approved")

  const isPending =
    updateRegistration.isPending || withdrawRegistration.isPending

  const participantLabel = getParticipantLabel(registration)

  function handleApprove() {
    updateRegistration.mutate({
      registrationId: registration.id,
      body: { status: "approved" },
    })
  }

  function handleReject() {
    updateRegistration.mutate(
      { registrationId: registration.id, body: { status: "rejected" } },
      { onSuccess: () => setConfirmReject(false) },
    )
  }

  function handleWithdraw() {
    withdrawRegistration.mutate(registration.id, {
      onSuccess: () => setConfirmWithdraw(false),
    })
  }

  return (
    <>
      <tr className="border-t border-border transition-colors hover:bg-accent/30">
        <td className="px-4 py-3">
          <div className="flex items-center gap-2.5">
            {/* Avatar placeholder */}
            <div
              className="flex size-7 shrink-0 items-center justify-center rounded-full bg-muted text-muted-foreground text-xs font-medium"
              aria-hidden="true"
            >
              {participantLabel.charAt(0).toUpperCase()}
            </div>
            <div className="min-w-0">
              <p className="truncate text-sm font-medium">{participantLabel}</p>
              {registration.notes && (
                <p className="truncate text-xs text-muted-foreground">{registration.notes}</p>
              )}
            </div>
          </div>
        </td>
        <td className="px-4 py-3">
          <StatusBadge status={registration.status} />
        </td>
        <td className="hidden px-4 py-3 text-muted-foreground sm:table-cell">
          {formatRelative(registration.registered_at)}
        </td>
        <td className="hidden px-4 py-3 text-muted-foreground md:table-cell">
          {registration.approved_at ? formatRelative(registration.approved_at) : "—"}
        </td>
        {canUpdate && (
          <td className="px-4 py-3 text-right">
            {!isTerminal && (
              <div className="flex items-center justify-end gap-1">
                {/* Quick approve */}
                {canApprove && (
                  <Button
                    variant="ghost"
                    size="icon"
                    className="size-8 text-green-600 hover:bg-green-50 hover:text-green-700 dark:hover:bg-green-950/40"
                    disabled={isPending}
                    onClick={handleApprove}
                    aria-label={`Approve ${participantLabel}`}
                    title={`Approve ${participantLabel}`}
                  >
                    {updateRegistration.isPending ? (
                      <Loader2Icon className="size-3.5 animate-spin" />
                    ) : (
                      <CheckIcon className="size-3.5" />
                    )}
                  </Button>
                )}

                {/* Quick reject */}
                {canReject && (
                  <Button
                    variant="ghost"
                    size="icon"
                    className="size-8 text-red-600 hover:bg-red-50 hover:text-red-700 dark:hover:bg-red-950/40"
                    disabled={isPending}
                    onClick={() => setConfirmReject(true)}
                    aria-label={`Reject ${participantLabel}`}
                    title={`Reject ${participantLabel}`}
                  >
                    <XIcon className="size-3.5" />
                  </Button>
                )}

                {/* More actions (withdraw) */}
                {canWithdraw && (
                  <DropdownMenu>
                    <DropdownMenuTrigger asChild>
                      <Button
                        variant="ghost"
                        size="icon"
                        className="size-8"
                        disabled={isPending}
                        aria-label={`More actions for ${participantLabel}`}
                      >
                        <MoreHorizontalIcon className="size-3.5" />
                      </Button>
                    </DropdownMenuTrigger>
                    <DropdownMenuContent align="end">
                      <DropdownMenuItem
                        className="text-destructive focus:text-destructive"
                        onClick={() => setConfirmWithdraw(true)}
                      >
                        <MinusCircleIcon className="mr-2 size-3.5" />
                        Withdraw
                      </DropdownMenuItem>
                    </DropdownMenuContent>
                  </DropdownMenu>
                )}
              </div>
            )}
          </td>
        )}
      </tr>

      {/* Reject confirmation */}
      <ConfirmDialog
        open={confirmReject}
        onOpenChange={setConfirmReject}
        title={`Reject ${participantLabel}?`}
        description="This registration will be rejected. The participant will not be able to register again for this tournament."
        confirmLabel="Reject"
        destructive
        isPending={updateRegistration.isPending}
        onConfirm={handleReject}
      />

      {/* Withdraw confirmation */}
      <ConfirmDialog
        open={confirmWithdraw}
        onOpenChange={setConfirmWithdraw}
        title={`Withdraw ${participantLabel}?`}
        description="This will withdraw the registration. This action cannot be undone."
        confirmLabel="Withdraw"
        destructive
        isPending={withdrawRegistration.isPending}
        onConfirm={handleWithdraw}
      />
    </>
  )
}

// ── Sub-components ────────────────────────────────────────────────────────────

function StatCard({
  label,
  value,
  description,
  colorClass,
}: {
  label: string
  value: number
  description?: string
  colorClass?: string
}) {
  return (
    <div className="rounded-xl border border-border bg-card px-4 py-3">
      <p className="text-xs text-muted-foreground">{label}</p>
      <p className={cn("text-2xl font-semibold tabular-nums", colorClass)}>{value}</p>
      {description && (
        <p className="text-[11px] leading-tight text-muted-foreground/70">{description}</p>
      )}
    </div>
  )
}

function RegistrationsTableSkeleton() {
  return (
    <div className="overflow-hidden rounded-xl border border-border" aria-busy="true">
      <table className="w-full text-sm">
        <thead>
          <tr className="bg-muted/40">
            <th className="px-4 py-2.5 text-left"><Skeleton className="h-4 w-20" /></th>
            <th className="px-4 py-2.5"><Skeleton className="h-4 w-16" /></th>
            <th className="hidden px-4 py-2.5 sm:table-cell"><Skeleton className="h-4 w-20" /></th>
            <th className="hidden px-4 py-2.5 md:table-cell"><Skeleton className="h-4 w-20" /></th>
          </tr>
        </thead>
        <tbody>
          {Array.from({ length: 5 }).map((_, i) => (
            <tr key={i} className="border-t border-border">
              <td className="px-4 py-3">
                <div className="flex items-center gap-2.5">
                  <Skeleton className="size-7 rounded-full" />
                  <Skeleton className="h-4 w-32" />
                </div>
              </td>
              <td className="px-4 py-3"><Skeleton className="h-5 w-20 rounded-full" /></td>
              <td className="hidden px-4 py-3 sm:table-cell"><Skeleton className="h-4 w-24" /></td>
              <td className="hidden px-4 py-3 md:table-cell"><Skeleton className="h-4 w-24" /></td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}
