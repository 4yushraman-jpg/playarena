"use client"

import * as React from "react"
import Link from "next/link"
import { useParams, useRouter, useSearchParams, usePathname } from "next/navigation"
import { type ColumnDef, type PaginationState, type SortingState } from "@tanstack/react-table"
import { PlusIcon, UsersIcon, SearchIcon, XIcon } from "lucide-react"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { PageHeader } from "@/components/ui/page-header"
import { DataTable } from "@/components/ui/data-table"
import { StatusBadge } from "@/components/ui/status-badge"
import { EmptyState } from "@/components/ui/empty-state"
import { AvatarDisplay } from "@/components/players/player-avatar"
import { usePlayerList } from "@/hooks/use-players"
import { useAuthStore, selectRole } from "@/stores/auth.store"
import { hasPermission } from "@/lib/permissions"
import { formatDate } from "@/lib/format"
import type { Player, PlayerStatus } from "@/types/api/players"

// ── Constants ──────────────────────────────────────────────────────────────────

const PAGE_SIZE = 20

const STATUS_OPTIONS: { value: string; label: string }[] = [
  { value: "all", label: "All statuses" },
  { value: "active", label: "Active" },
  { value: "inactive", label: "Inactive" },
  { value: "injured", label: "Injured" },
  { value: "suspended", label: "Suspended" },
  { value: "retired", label: "Retired" },
]

// ── Columns ───────────────────────────────────────────────────────────────────

function buildColumns(orgSlug: string): ColumnDef<Player, unknown>[] {
  return [
    {
      id: "player",
      header: "Player",
      cell: ({ row }) => {
        const p = row.original
        return (
          <Link
            href={`/${orgSlug}/players/${p.id}`}
            className="flex items-center gap-2.5 hover:underline"
          >
            <AvatarDisplay displayName={p.display_name} size="sm" />
            <span className="font-medium">{p.display_name}</span>
          </Link>
        )
      },
    },
    {
      accessorKey: "jersey_number",
      header: "#",
      cell: ({ getValue }) => {
        const v = getValue() as string | null
        return v ? <span className="font-mono text-sm">#{v}</span> : <span className="text-muted-foreground">—</span>
      },
    },
    {
      accessorKey: "position",
      header: "Position",
      cell: ({ getValue }) => {
        const v = getValue() as string | null
        return v ?? <span className="text-muted-foreground">—</span>
      },
    },
    {
      accessorKey: "status",
      header: "Status",
      cell: ({ getValue }) => <StatusBadge status={getValue() as PlayerStatus} />,
    },
    {
      accessorKey: "created_at",
      header: "Joined",
      enableSorting: true,
      cell: ({ getValue }) => (
        <span className="text-muted-foreground">{formatDate(getValue() as string)}</span>
      ),
    },
  ]
}

// ── Page ──────────────────────────────────────────────────────────────────────

export default function PlayersPage() {
  const params = useParams<{ orgSlug: string }>()
  const orgSlug = params.orgSlug
  const role = useAuthStore(selectRole)
  const router = useRouter()
  const pathname = usePathname()
  const searchParams = useSearchParams()

  // ── URL state ──────────────────────────────────────────────────────────────

  const searchQuery = searchParams.get("q") ?? ""
  const statusFilter = (searchParams.get("status") ?? "all") as PlayerStatus | "all"
  const pageIndex = Math.max(0, Number(searchParams.get("page") ?? "0"))

  // Derive pagination directly from URL to avoid setState-in-effect
  const pagination: PaginationState = { pageIndex, pageSize: PAGE_SIZE }
  const [sorting, setSorting] = React.useState<SortingState>([])
  const [localSearch, setLocalSearch] = React.useState(searchQuery)

  // Sync the search input when the URL changes via browser back/forward.
  // searchQuery is read from useSearchParams (external, read-only) so updating
  // localSearch here cannot create a cycle — the effect only fires on genuine
  // URL navigations, not on user typing.
  // eslint-disable-next-line react-hooks/set-state-in-effect
  React.useEffect(() => { setLocalSearch(searchQuery) }, [searchQuery])

  function updateUrl(updates: Record<string, string | null>) {
    const sp = new URLSearchParams(searchParams.toString())
    for (const [k, v] of Object.entries(updates)) {
      if (v === null || v === "" || v === "all" || v === "0") {
        sp.delete(k)
      } else {
        sp.set(k, v)
      }
    }
    router.replace(`${pathname}?${sp.toString()}`, { scroll: false })
  }

  // Debounce search input
  React.useEffect(() => {
    const id = setTimeout(() => {
      updateUrl({ q: localSearch, page: null })
    }, 350)
    return () => clearTimeout(id)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [localSearch])

  function handleStatusChange(v: string) {
    updateUrl({ status: v, page: null })
  }

  function handlePaginationChange(updater: React.SetStateAction<PaginationState>) {
    const next = typeof updater === "function" ? updater(pagination) : updater
    updateUrl({ page: next.pageIndex === 0 ? null : String(next.pageIndex) })
  }

  function clearFilters() {
    setLocalSearch("")
    updateUrl({ q: null, status: null, page: null })
  }

  // ── Query ──────────────────────────────────────────────────────────────────

  const queryParams = {
    limit: PAGE_SIZE,
    offset: pagination.pageIndex * PAGE_SIZE,
    search: searchQuery || undefined,
    status: statusFilter !== "all" ? statusFilter : undefined,
  }

  const { data, isLoading, isError, refetch } = usePlayerList(orgSlug, queryParams)

  const canCreate = hasPermission(role, "player.create")
  const hasFilters = !!localSearch || statusFilter !== "all"
  const columns = React.useMemo(() => buildColumns(orgSlug), [orgSlug])

  return (
    <div className="space-y-6">
      <PageHeader
        title="Players"
        description="Manage your organization's player roster."
        breadcrumbs={[{ label: "Dashboard", href: `/${orgSlug}` }, { label: "Players" }]}
        action={
          canCreate ? (
            <Button asChild size="sm" className="gap-1.5">
              <Link href={`/${orgSlug}/players/new`}>
                <PlusIcon className="size-3.5" />
                New player
              </Link>
            </Button>
          ) : undefined
        }
      />

      {/* Filters */}
      <div className="flex flex-wrap items-center gap-3">
        <div className="relative flex-1 min-w-48 max-w-sm">
          <SearchIcon className="absolute left-2.5 top-1/2 size-3.5 -translate-y-1/2 text-muted-foreground" />
          <Input
            placeholder="Search players…"
            value={localSearch}
            onChange={(e) => setLocalSearch(e.target.value)}
            className="pl-8"
            aria-label="Search players"
          />
        </div>

        <Select value={statusFilter} onValueChange={handleStatusChange}>
          <SelectTrigger className="w-40" aria-label="Filter by status">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {STATUS_OPTIONS.map((o) => (
              <SelectItem key={o.value} value={o.value}>
                {o.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>

        {hasFilters && (
          <Button variant="ghost" size="sm" onClick={clearFilters} className="gap-1.5 text-muted-foreground">
            <XIcon className="size-3.5" />
            Clear
          </Button>
        )}

        {data && (
          <span className="ml-auto text-sm text-muted-foreground">
            {data.total.toLocaleString()} {data.total === 1 ? "player" : "players"}
          </span>
        )}
      </div>

      {/* Error state */}
      {isError ? (
        <div className="flex flex-col items-center gap-3 rounded-xl border border-dashed border-border py-16 text-center">
          <p className="text-sm font-medium">Failed to load players</p>
          <p className="text-xs text-muted-foreground">Check your connection and try again.</p>
          <Button variant="outline" size="sm" onClick={() => refetch()}>
            Retry
          </Button>
        </div>
      ) : !isLoading && data?.total === 0 && !hasFilters ? (
        /* First-time empty state */
        <EmptyState
          icon={<UsersIcon />}
          title="No players yet"
          description="Add your first player to get started. Players can be assigned to teams, tournaments, and matches."
          action={
            canCreate ? (
              <Button asChild size="sm" className="gap-1.5">
                <Link href={`/${orgSlug}/players/new`}>
                  <PlusIcon className="size-3.5" />
                  Add first player
                </Link>
              </Button>
            ) : undefined
          }
        />
      ) : !isLoading && data?.total === 0 && hasFilters ? (
        /* Filtered empty state */
        <EmptyState
          icon={<SearchIcon />}
          title="No players match your filters"
          description="Try adjusting your search or clearing the filters."
          action={
            <Button variant="outline" size="sm" onClick={clearFilters}>
              Clear filters
            </Button>
          }
        />
      ) : (
        <DataTable
          columns={columns}
          data={data?.players ?? []}
          total={data?.total}
          isLoading={isLoading}
          pagination={pagination}
          onPaginationChange={handlePaginationChange}
          sorting={sorting}
          onSortingChange={setSorting}
        />
      )}
    </div>
  )
}
