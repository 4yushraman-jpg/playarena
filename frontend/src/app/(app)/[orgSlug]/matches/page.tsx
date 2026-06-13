"use client"

import { Suspense, useCallback, useEffect, useMemo, useState } from "react"
import Link from "next/link"
import { useParams, usePathname, useRouter, useSearchParams } from "next/navigation"
import { SwordsIcon, SearchIcon, XIcon } from "lucide-react"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Skeleton } from "@/components/ui/skeleton"
import { StatusBadge } from "@/components/ui/status-badge"
import { EmptyState } from "@/components/ui/empty-state"
import { PageHeader } from "@/components/ui/page-header"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { useMatchList } from "@/hooks/use-matches"
import { useTournamentList } from "@/hooks/use-tournaments"
import { useParticipantNames } from "@/hooks/use-participant-names"
import { useDebouncedValue } from "@/hooks/use-debounce"
import {
  parseMatchListState,
  serializeMatchListState,
  type MatchListState,
} from "@/lib/match-list-state"
import { formatMatchLabel, matchParticipantIds } from "@/lib/match-meta"
import { formatDateTime, formatScore } from "@/lib/format"
import type { Match, MatchStatus } from "@/types/api/matches"

const STATUS_OPTIONS: { value: MatchStatus | "all"; label: string }[] = [
  { value: "all", label: "All statuses" },
  { value: "scheduled", label: "Scheduled" },
  { value: "live", label: "Live" },
  { value: "completed", label: "Completed" },
  { value: "cancelled", label: "Cancelled" },
  { value: "abandoned", label: "Abandoned" },
]

const PAGE_SIZE = 20
const SEARCH_DEBOUNCE_MS = 300

export default function MatchesPage() {
  return (
    <Suspense fallback={<DirectoryFallback />}>
      <MatchesDirectory />
    </Suspense>
  )
}

function MatchesDirectory() {
  const params = useParams<{ orgSlug: string }>()
  const orgSlug = params.orgSlug
  const router = useRouter()
  const pathname = usePathname()
  const searchParams = useSearchParams()

  const state = parseMatchListState(searchParams)
  const [searchInput, setSearchInput] = useState(state.search)
  const debouncedSearch = useDebouncedValue(searchInput, SEARCH_DEBOUNCE_MS)

  const applyState = useCallback(
    (next: MatchListState) => {
      const qs = serializeMatchListState(next)
      router.replace(qs ? `${pathname}?${qs}` : pathname, { scroll: false })
    },
    [router, pathname],
  )

  useEffect(() => {
    if (debouncedSearch !== state.search) {
      applyState({ ...state, search: debouncedSearch, page: 0 })
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps -- state fields are derived from the URL each render
  }, [debouncedSearch])

  const queryParams = {
    limit: PAGE_SIZE,
    offset: state.page * PAGE_SIZE,
    search: state.search || undefined,
    status: state.status !== "all" ? state.status : undefined,
  }

  const { data, isLoading, isError, refetch } = useMatchList(orgSlug, queryParams)
  const { resolve } = useParticipantNames(orgSlug)

  // Tournaments are few; one bounded fetch resolves the host name per fixture.
  const tournamentsQuery = useTournamentList(orgSlug, { limit: 100 })
  const tournamentNames = useMemo(() => {
    const map = new Map<string, string>()
    for (const t of tournamentsQuery.data?.tournaments ?? []) map.set(t.id, t.name)
    return map
  }, [tournamentsQuery.data])

  const matches = data?.matches ?? []
  const total = data?.total ?? 0
  const totalPages = Math.ceil(total / PAGE_SIZE)

  function handleStatusChange(value: string) {
    applyState({ ...state, status: value as MatchStatus | "all", page: 0 })
  }

  function clearFilters() {
    setSearchInput("")
    applyState({ search: "", status: "all", page: 0 })
  }

  const hasFilters = state.search !== "" || state.status !== "all"

  return (
    <div className="space-y-6">
      <PageHeader
        title="Matches"
        description="Browse and manage fixtures across your tournaments."
        breadcrumbs={[
          { label: "Dashboard", href: `/${orgSlug}` },
          { label: "Matches" },
        ]}
      />

      {/* Filters */}
      <div className="flex flex-wrap items-center gap-3">
        <div className="relative min-w-0 flex-1 sm:max-w-xs">
          <SearchIcon className="pointer-events-none absolute left-2.5 top-1/2 size-3.5 -translate-y-1/2 text-muted-foreground" />
          <Input
            placeholder="Search by venue or round…"
            value={searchInput}
            onChange={(e) => setSearchInput(e.target.value)}
            className="pl-8 pr-8"
            aria-label="Search matches by venue or round"
          />
          {searchInput && (
            <button
              className="absolute right-2.5 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground"
              onClick={() => setSearchInput("")}
              aria-label="Clear search"
            >
              <XIcon className="size-3.5" />
            </button>
          )}
        </div>
        <Select value={state.status} onValueChange={handleStatusChange}>
          <SelectTrigger className="w-44" aria-label="Filter by status">
            <SelectValue placeholder="Status" />
          </SelectTrigger>
          <SelectContent>
            {STATUS_OPTIONS.map((opt) => (
              <SelectItem key={opt.value} value={opt.value}>
                {opt.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        {hasFilters && (
          <Button variant="ghost" size="sm" onClick={clearFilters}>
            Clear filters
          </Button>
        )}
      </div>

      <MatchTable
        matches={matches}
        isLoading={isLoading}
        isError={isError}
        orgSlug={orgSlug}
        onRetry={refetch}
        hasFilters={hasFilters}
        resolveName={resolve}
        tournamentNames={tournamentNames}
      />

      {totalPages > 1 && (
        <nav
          className="flex items-center justify-between text-sm text-muted-foreground"
          aria-label="Match pages"
        >
          <span>
            {total} match{total !== 1 ? "es" : ""}
          </span>
          <div className="flex items-center gap-2">
            <Button
              variant="outline"
              size="sm"
              className="min-h-9 px-4"
              disabled={state.page === 0}
              onClick={() => applyState({ ...state, page: state.page - 1 })}
            >
              Previous
            </Button>
            <span className="tabular-nums" aria-live="polite">
              {state.page + 1} / {totalPages}
            </span>
            <Button
              variant="outline"
              size="sm"
              className="min-h-9 px-4"
              disabled={state.page >= totalPages - 1}
              onClick={() => applyState({ ...state, page: state.page + 1 })}
            >
              Next
            </Button>
          </div>
        </nav>
      )}
    </div>
  )
}

// ── Table ──────────────────────────────────────────────────────────────────────

interface MatchTableProps {
  matches: Match[]
  isLoading: boolean
  isError: boolean
  orgSlug: string
  onRetry: () => void
  hasFilters: boolean
  resolveName: (teamId: string | null, playerId: string | null) => string
  tournamentNames: Map<string, string>
}

function MatchTable({
  matches,
  isLoading,
  isError,
  orgSlug,
  onRetry,
  hasFilters,
  resolveName,
  tournamentNames,
}: MatchTableProps) {
  if (isLoading) {
    return (
      <div className="overflow-hidden rounded-xl border border-border" aria-busy="true" aria-label="Loading matches">
        <table className="w-full text-sm">
          <thead>
            <THead />
          </thead>
          <tbody>
            {Array.from({ length: 5 }).map((_, i) => (
              <tr key={i} className="border-t border-border">
                <td className="px-4 py-3"><Skeleton className="h-4 w-44" /></td>
                <td className="px-4 py-3"><Skeleton className="h-5 w-24 rounded-full" /></td>
                <td className="hidden px-4 py-3 md:table-cell"><Skeleton className="h-4 w-32" /></td>
                <td className="hidden px-4 py-3 lg:table-cell"><Skeleton className="h-4 w-28" /></td>
                <td className="px-4 py-3"><Skeleton className="h-4 w-12" /></td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    )
  }

  if (isError) {
    return (
      <EmptyState
        icon={<SwordsIcon />}
        title="Failed to load matches"
        description="Something went wrong. Please try again."
        action={
          <Button variant="outline" size="sm" onClick={onRetry}>
            Retry
          </Button>
        }
      />
    )
  }

  if (matches.length === 0) {
    return (
      <EmptyState
        icon={<SwordsIcon />}
        title={hasFilters ? "No matches match your filters" : "No matches yet"}
        description={
          hasFilters
            ? "Try a different venue or round, or adjust the status filter. Searching by participant name isn't supported here — open a tournament to see its fixtures."
            : "Fixtures created in your ongoing tournaments appear here."
        }
      />
    )
  }

  return (
    <div className="overflow-hidden rounded-xl border border-border">
      <div className="overflow-x-auto">
        <table className="w-full text-sm" aria-label="Matches">
          <thead>
            <THead />
          </thead>
          <tbody>
            {matches.map((m) => {
              const { homeId, awayId } = matchParticipantIds(m)
              const isTeam = !!(m.home_team_id || m.away_team_id)
              const homeName = resolveName(isTeam ? homeId : null, isTeam ? null : homeId)
              const awayName = resolveName(isTeam ? awayId : null, isTeam ? null : awayId)
              const tournamentName = tournamentNames.get(m.tournament_id)
              return (
                <tr
                  key={m.id}
                  className="border-t border-border transition-colors hover:bg-accent/40"
                >
                  <td className="px-4 py-3">
                    <Link
                      href={`/${orgSlug}/matches/${m.id}`}
                      className="font-medium text-foreground hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:rounded"
                    >
                      {homeName} <span className="text-muted-foreground">vs</span> {awayName}
                    </Link>
                    <div className="mt-0.5 truncate text-xs text-muted-foreground">
                      {formatMatchLabel(m)}
                    </div>
                  </td>
                  <td className="px-4 py-3">
                    <StatusBadge status={m.status} />
                  </td>
                  <td className="hidden px-4 py-3 md:table-cell">
                    {tournamentName ? (
                      <Link
                        href={`/${orgSlug}/tournaments/${m.tournament_id}`}
                        className="text-muted-foreground hover:text-foreground hover:underline"
                      >
                        {tournamentName}
                      </Link>
                    ) : (
                      <span className="text-muted-foreground">—</span>
                    )}
                  </td>
                  <td className="hidden px-4 py-3 lg:table-cell">
                    {m.scheduled_at ? (
                      formatDateTime(m.scheduled_at)
                    ) : (
                      <span className="text-muted-foreground">—</span>
                    )}
                  </td>
                  <td className="px-4 py-3 text-right tabular-nums font-medium">
                    {m.status === "completed" ? (
                      formatScore(m.home_score, m.away_score)
                    ) : (
                      <span className="text-muted-foreground">—</span>
                    )}
                  </td>
                </tr>
              )
            })}
          </tbody>
        </table>
      </div>
    </div>
  )
}

function THead() {
  return (
    <tr className="bg-muted/40 text-left text-xs font-medium text-muted-foreground">
      <th scope="col" className="px-4 py-2.5">Match</th>
      <th scope="col" className="px-4 py-2.5">Status</th>
      <th scope="col" className="hidden px-4 py-2.5 md:table-cell">Tournament</th>
      <th scope="col" className="hidden px-4 py-2.5 lg:table-cell">Scheduled</th>
      <th scope="col" className="px-4 py-2.5 text-right">Score</th>
    </tr>
  )
}

function DirectoryFallback() {
  return (
    <div className="space-y-6" aria-busy="true" aria-label="Loading matches">
      <div className="space-y-2">
        <Skeleton className="h-4 w-48" />
        <Skeleton className="h-8 w-56" />
      </div>
      <div className="flex gap-3">
        <Skeleton className="h-9 w-64" />
        <Skeleton className="h-9 w-44" />
      </div>
      <Skeleton className="h-72 w-full rounded-xl" />
    </div>
  )
}
