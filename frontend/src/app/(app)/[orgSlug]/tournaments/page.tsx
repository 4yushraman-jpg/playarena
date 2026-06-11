"use client"

import { Suspense, useCallback, useEffect, useState } from "react"
import Link from "next/link"
import { useParams, usePathname, useRouter, useSearchParams } from "next/navigation"
import { PlusIcon, TrophyIcon, SearchIcon, XIcon } from "lucide-react"
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
import { useTournamentList } from "@/hooks/use-tournaments"
import { useDebouncedValue } from "@/hooks/use-debounce"
import {
  parseListState,
  serializeListState,
  type TournamentListState,
} from "@/lib/tournament-list-state"
import { formatCapacityLabel } from "@/lib/registration-stats"
import type { Tournament, TournamentStatus } from "@/types/api/tournaments"
import { useAuthStore, selectRole } from "@/stores/auth.store"
import { hasPermission } from "@/lib/permissions"
import { formatDate, formatPrizePool } from "@/lib/format"

// ── Constants ─────────────────────────────────────────────────────────────────

const STATUS_OPTIONS: { value: TournamentStatus | "all"; label: string }[] = [
  { value: "all", label: "All statuses" },
  { value: "draft", label: "Draft" },
  { value: "registration_open", label: "Registration open" },
  { value: "registration_closed", label: "Reg. closed" },
  { value: "ongoing", label: "Ongoing" },
  { value: "completed", label: "Completed" },
  { value: "cancelled", label: "Cancelled" },
]

const PAGE_SIZE = 20
const SEARCH_DEBOUNCE_MS = 300

// ── Page ──────────────────────────────────────────────────────────────────────

export default function TournamentsPage() {
  return (
    <Suspense fallback={<DirectoryFallback />}>
      <TournamentsDirectory />
    </Suspense>
  )
}

function TournamentsDirectory() {
  const params = useParams<{ orgSlug: string }>()
  const orgSlug = params.orgSlug
  const router = useRouter()
  const pathname = usePathname()
  const searchParams = useSearchParams()
  const role = useAuthStore(selectRole)
  const canCreate = hasPermission(role, "tournament.create")

  // The URL is the source of truth so refreshes and shared links restore the
  // exact view. The search box keeps a local mirror for instant keystrokes;
  // the debounced value is what hits the URL (and therefore the API).
  const state = parseListState(searchParams)
  const [searchInput, setSearchInput] = useState(state.search)
  const debouncedSearch = useDebouncedValue(searchInput, SEARCH_DEBOUNCE_MS)

  const applyState = useCallback(
    (next: TournamentListState) => {
      const qs = serializeListState(next)
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

  const { data, isLoading, isError, refetch } = useTournamentList(orgSlug, queryParams)

  const tournaments = data?.tournaments ?? []
  const total = data?.total ?? 0
  const totalPages = Math.ceil(total / PAGE_SIZE)

  function handleStatusChange(value: string) {
    applyState({ ...state, status: value as TournamentStatus | "all", page: 0 })
  }

  function clearFilters() {
    setSearchInput("")
    applyState({ search: "", status: "all", page: 0 })
  }

  const hasFilters = state.search !== "" || state.status !== "all"

  return (
    <div className="space-y-6">
      <PageHeader
        title="Tournaments"
        description="Manage and discover tournaments in your organization."
        breadcrumbs={[
          { label: "Dashboard", href: `/${orgSlug}` },
          { label: "Tournaments" },
        ]}
        action={
          canCreate ? (
            <Button asChild size="sm" className="gap-2">
              <Link href={`/${orgSlug}/tournaments/new`}>
                <PlusIcon className="size-3.5" />
                New tournament
              </Link>
            </Button>
          ) : undefined
        }
      />

      {/* Filters */}
      <div className="flex flex-wrap items-center gap-3">
        <div className="relative min-w-0 flex-1 sm:max-w-xs">
          <SearchIcon className="pointer-events-none absolute left-2.5 top-1/2 size-3.5 -translate-y-1/2 text-muted-foreground" />
          <Input
            placeholder="Search tournaments…"
            value={searchInput}
            onChange={(e) => setSearchInput(e.target.value)}
            className="pl-8 pr-8"
            aria-label="Search tournaments"
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

      {/* Table */}
      <TournamentTable
        tournaments={tournaments}
        isLoading={isLoading}
        isError={isError}
        orgSlug={orgSlug}
        onRetry={refetch}
        hasFilters={hasFilters}
        canCreate={canCreate}
      />

      {/* Pagination */}
      {totalPages > 1 && (
        <nav
          className="flex items-center justify-between text-sm text-muted-foreground"
          aria-label="Tournament pages"
        >
          <span>
            {total} tournament{total !== 1 ? "s" : ""}
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

// ── Table component ───────────────────────────────────────────────────────────

interface TournamentTableProps {
  tournaments: Tournament[]
  isLoading: boolean
  isError: boolean
  orgSlug: string
  onRetry: () => void
  hasFilters: boolean
  canCreate: boolean
}

function TournamentTable({
  tournaments,
  isLoading,
  isError,
  orgSlug,
  onRetry,
  hasFilters,
  canCreate,
}: TournamentTableProps) {
  if (isLoading) {
    return (
      <div className="overflow-hidden rounded-xl border border-border" aria-busy="true" aria-label="Loading tournaments">
        <table className="w-full text-sm">
          <thead>
            <THead />
          </thead>
          <tbody>
            {Array.from({ length: 5 }).map((_, i) => (
              <tr key={i} className="border-t border-border">
                <td className="px-4 py-3"><Skeleton className="h-4 w-40" /></td>
                <td className="px-4 py-3"><Skeleton className="h-5 w-28 rounded-full" /></td>
                <td className="hidden px-4 py-3 sm:table-cell"><Skeleton className="h-4 w-16" /></td>
                <td className="hidden px-4 py-3 md:table-cell"><Skeleton className="h-4 w-24" /></td>
                <td className="hidden px-4 py-3 lg:table-cell"><Skeleton className="h-4 w-20" /></td>
                <td className="hidden px-4 py-3 xl:table-cell"><Skeleton className="h-4 w-24" /></td>
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
        icon={<TrophyIcon />}
        title="Failed to load tournaments"
        description="Something went wrong. Please try again."
        action={
          <Button variant="outline" size="sm" onClick={onRetry}>
            Retry
          </Button>
        }
      />
    )
  }

  if (tournaments.length === 0) {
    return (
      <EmptyState
        icon={<TrophyIcon />}
        title={hasFilters ? "No tournaments match your filters" : "No tournaments yet"}
        description={
          hasFilters
            ? "Try adjusting your search or status filter."
            : canCreate
            ? "Create your first tournament to get started."
            : "Tournaments created by your organization appear here."
        }
        action={
          !hasFilters && canCreate ? (
            <Button asChild size="sm" className="gap-2">
              <Link href={`/${orgSlug}/tournaments/new`}>
                <PlusIcon className="size-3.5" />
                New tournament
              </Link>
            </Button>
          ) : undefined
        }
      />
    )
  }

  return (
    <div className="overflow-hidden rounded-xl border border-border">
      <div className="overflow-x-auto">
        <table className="w-full text-sm" aria-label="Tournaments">
          <thead>
            <THead />
          </thead>
          <tbody>
            {tournaments.map((t) => (
              <tr
                key={t.id}
                className="border-t border-border transition-colors hover:bg-accent/40"
              >
                <td className="px-4 py-3">
                  <Link
                    href={`/${orgSlug}/tournaments/${t.id}`}
                    className="font-medium text-foreground hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:rounded"
                  >
                    {t.name}
                  </Link>
                  <div className="mt-0.5 flex items-center gap-1.5 text-xs text-muted-foreground">
                    <span className="capitalize">{t.sport}</span>
                    <span aria-hidden="true">·</span>
                    <span className="capitalize">{t.format.replace(/_/g, " ")}</span>
                  </div>
                </td>
                <td className="px-4 py-3">
                  <StatusBadge status={t.status} />
                </td>
                <td className="hidden px-4 py-3 sm:table-cell">
                  <span className="capitalize">{t.participant_type}</span>
                </td>
                <td className="hidden px-4 py-3 md:table-cell">
                  {t.starts_at ? formatDate(t.starts_at) : <span className="text-muted-foreground">—</span>}
                </td>
                <td className="hidden px-4 py-3 lg:table-cell tabular-nums">
                  {formatCapacityLabel(t)}
                </td>
                <td className="hidden px-4 py-3 xl:table-cell">
                  {t.prize_pool ? (
                    formatPrizePool(t.prize_pool)
                  ) : (
                    <span className="text-muted-foreground">—</span>
                  )}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  )
}

function THead() {
  return (
    <tr className="bg-muted/40 text-left text-xs font-medium text-muted-foreground">
      <th scope="col" className="px-4 py-2.5">Tournament</th>
      <th scope="col" className="px-4 py-2.5">Status</th>
      <th scope="col" className="hidden px-4 py-2.5 sm:table-cell">Type</th>
      <th scope="col" className="hidden px-4 py-2.5 md:table-cell">Starts</th>
      <th scope="col" className="hidden px-4 py-2.5 lg:table-cell">Registered</th>
      <th scope="col" className="hidden px-4 py-2.5 xl:table-cell">Prize pool</th>
    </tr>
  )
}

function DirectoryFallback() {
  return (
    <div className="space-y-6" aria-busy="true" aria-label="Loading tournaments">
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
