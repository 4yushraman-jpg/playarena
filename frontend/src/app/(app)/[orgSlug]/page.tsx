"use client"

import Link from "next/link"
import { useParams } from "next/navigation"
import { useQuery } from "@tanstack/react-query"
import {
  TrophyIcon,
  SwordsIcon,
  BellIcon,
  BarChart2Icon,
  ChevronRightIcon,
  AlertCircleIcon,
  ZapIcon,
} from "lucide-react"
import { Button } from "@/components/ui/button"
import { Badge } from "@/components/ui/badge"
import { Skeleton } from "@/components/ui/skeleton"
import { EmptyState } from "@/components/ui/empty-state"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { StatusBadge } from "@/components/ui/status-badge"
import { useAuthStore, selectRole } from "@/stores/auth.store"
import { useUnreadCount } from "@/hooks/use-unread-count"
import { useCurrentUser } from "@/hooks/use-current-user"
import { notificationKeys, tournamentKeys, matchKeys } from "@/lib/query-keys"
import { notificationsApi } from "@/lib/api/notifications"
import { tournamentsApi } from "@/lib/api/tournaments"
import { matchesApi } from "@/lib/api/matches"
import { hasPermission, ROLE_LABELS, ROLE_VARIANTS } from "@/lib/permissions"
import { getNotificationLabel } from "@/lib/notification-copy"
import { formatRelative, formatDateTime } from "@/lib/format"
import { cn } from "@/lib/utils"
import type { Permission } from "@/types/common"
import type { TournamentStatus } from "@/types/api/tournaments"

// ── Quick actions ─────────────────────────────────────────────────────────────

interface QuickAction {
  label: string
  href: string
  icon: React.ElementType
  permission?: Permission
}

function getQuickActions(orgSlug: string, role: ReturnType<typeof selectRole>): QuickAction[] {
  const base = `/${orgSlug}`
  const all: QuickAction[] = [
    {
      label: "New Tournament",
      href: `${base}/tournaments/new`,
      icon: TrophyIcon,
      permission: "tournament.create",
    },
    {
      label: "New Match",
      href: `${base}/matches`,
      icon: SwordsIcon,
      permission: "match.create",
    },
    {
      label: "Rankings",
      href: `${base}/rankings`,
      icon: BarChart2Icon,
    },
    {
      label: "Notifications",
      href: `${base}/notifications`,
      icon: BellIcon,
    },
  ]
  return all.filter((a) => !a.permission || hasPermission(role, a.permission))
}

// ── Dashboard page ────────────────────────────────────────────────────────────

export default function DashboardPage() {
  const params = useParams<{ orgSlug: string }>()
  const orgSlug = params.orgSlug
  const role = useAuthStore(selectRole)
  const { data: user } = useCurrentUser()
  const { unreadCount } = useUnreadCount(orgSlug)

  const { data: notifData, isLoading: notifLoading, isError: notifError, refetch: refetchNotif } = useQuery({
    queryKey: notificationKeys.list(orgSlug, { limit: 5, offset: 0 }),
    queryFn: () =>
      notificationsApi.list(orgSlug, { limit: 5, offset: 0 }).then((r) => r.data),
    staleTime: 30_000,
  })

  const { data: tournamentData, isLoading: tournamentLoading, isError: tournamentError, refetch: refetchTournaments } = useQuery({
    queryKey: tournamentKeys.list(orgSlug, { limit: 5, status: "registration_open" as TournamentStatus }),
    queryFn: () =>
      tournamentsApi
        .list(orgSlug, { limit: 5, status: "registration_open" })
        .then((r) => r.data),
    staleTime: 60_000,
  })

  const { data: matchData, isLoading: matchLoading, isError: matchError, refetch: refetchMatches } = useQuery({
    queryKey: matchKeys.list(orgSlug, { limit: 5 }),
    queryFn: () =>
      matchesApi.list(orgSlug, { limit: 5 }).then((r) => r.data),
    staleTime: 30_000,
  })

  const quickActions = getQuickActions(orgSlug, role)
  const firstName = user?.full_name || user?.username || "there"

  return (
    <div className="space-y-8">
      {/* Welcome banner */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div className="space-y-1">
          <div className="flex flex-wrap items-center gap-2">
            <h1 className="text-2xl font-semibold tracking-tight">
              Welcome back, {firstName}
            </h1>
            {role && (
              <Badge variant={ROLE_VARIANTS[role]} className="capitalize">
                {ROLE_LABELS[role]}
              </Badge>
            )}
          </div>
          <p className="text-sm text-muted-foreground">
            Here&apos;s what&apos;s happening in your organization.
          </p>
        </div>
        {unreadCount > 0 && (
          <Button asChild variant="outline" size="sm" className="gap-2 shrink-0">
            <Link href={`/${orgSlug}/notifications`}>
              <BellIcon className="size-3.5" />
              {unreadCount} unread
            </Link>
          </Button>
        )}
      </div>

      {/* Quick actions */}
      {quickActions.length > 0 && (
        <section aria-labelledby="quick-actions-heading">
          <h2
            id="quick-actions-heading"
            className="mb-3 text-sm font-medium text-muted-foreground"
          >
            Quick actions
          </h2>
          <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
            {quickActions.map((action) => (
              <Link
                key={action.href}
                href={action.href}
                className="group flex flex-col items-center gap-2 rounded-xl border border-border bg-card p-4 text-center transition-colors hover:border-primary/30 hover:bg-accent/40 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
              >
                <div className="flex size-9 items-center justify-center rounded-lg bg-primary/10 text-primary transition-colors group-hover:bg-primary/20">
                  <action.icon className="size-4" />
                </div>
                <span className="text-xs font-medium">{action.label}</span>
              </Link>
            ))}
          </div>
        </section>
      )}

      {/* Widgets row */}
      <div className="grid gap-6 lg:grid-cols-3">
        {/* Recent Notifications */}
        <Card>
          <CardHeader className="flex-row items-center justify-between pb-2">
            <CardTitle className="flex items-center gap-2">
              <BellIcon className="size-4 text-muted-foreground" />
              Notifications
              {unreadCount > 0 && (
                <Badge variant="default" className="h-5 min-w-5 rounded-full px-1.5 text-xs">
                  {unreadCount}
                </Badge>
              )}
            </CardTitle>
            <Link
              href={`/${orgSlug}/notifications`}
              className="flex items-center gap-0.5 text-xs text-muted-foreground hover:text-foreground"
            >
              View all <ChevronRightIcon className="size-3" />
            </Link>
          </CardHeader>
          <CardContent>
            <NotificationsWidget
              isLoading={notifLoading}
              isError={notifError}
              notifications={notifData?.notifications ?? []}
              onRetry={refetchNotif}
            />
          </CardContent>
        </Card>

        {/* Upcoming Tournaments */}
        <Card>
          <CardHeader className="flex-row items-center justify-between pb-2">
            <CardTitle className="flex items-center gap-2">
              <TrophyIcon className="size-4 text-muted-foreground" />
              Tournaments
            </CardTitle>
            <Link
              href={`/${orgSlug}/tournaments`}
              className="flex items-center gap-0.5 text-xs text-muted-foreground hover:text-foreground"
            >
              View all <ChevronRightIcon className="size-3" />
            </Link>
          </CardHeader>
          <CardContent>
            <TournamentsWidget
              isLoading={tournamentLoading}
              isError={tournamentError}
              tournaments={tournamentData?.tournaments ?? []}
              orgSlug={orgSlug}
              onRetry={refetchTournaments}
            />
          </CardContent>
        </Card>

        {/* Recent Matches */}
        <Card>
          <CardHeader className="flex-row items-center justify-between pb-2">
            <CardTitle className="flex items-center gap-2">
              <SwordsIcon className="size-4 text-muted-foreground" />
              Recent Matches
            </CardTitle>
            <Link
              href={`/${orgSlug}/matches`}
              className="flex items-center gap-0.5 text-xs text-muted-foreground hover:text-foreground"
            >
              View all <ChevronRightIcon className="size-3" />
            </Link>
          </CardHeader>
          <CardContent>
            <MatchesWidget
              isLoading={matchLoading}
              isError={matchError}
              matches={matchData?.matches ?? []}
              onRetry={refetchMatches}
            />
          </CardContent>
        </Card>
      </div>
    </div>
  )
}

// ── Widget components ─────────────────────────────────────────────────────────

interface NotificationsWidgetProps {
  isLoading: boolean
  isError: boolean
  notifications: import("@/types/api/notifications").Notification[]
  onRetry?: () => void
}

function NotificationsWidget({ isLoading, isError, notifications, onRetry }: NotificationsWidgetProps) {
  if (isLoading) return <WidgetSkeleton rows={4} />
  if (isError) return <WidgetError onRetry={onRetry} />
  if (notifications.length === 0) {
    return (
      <EmptyState
        icon={<BellIcon />}
        title="No notifications yet"
        description="You&apos;ll see activity here."
        className="min-h-32 border-0 bg-transparent py-6"
      />
    )
  }
  return (
    <ul className="space-y-3" role="list">
      {notifications.map((n) => (
        <li key={n.id} className="flex items-start gap-2.5">
          <span
            className={cn(
              "mt-1.5 block size-1.5 shrink-0 rounded-full",
              n.read_at === null ? "bg-primary" : "bg-muted-foreground/30",
            )}
            aria-hidden="true"
          />
          <div className="min-w-0 flex-1">
            <p className={cn("truncate text-xs", n.read_at === null ? "font-medium" : "text-muted-foreground")}>
              {getNotificationLabel(n.event_type)}
            </p>
            <p className="text-xs text-muted-foreground/70">{formatRelative(n.created_at)}</p>
          </div>
        </li>
      ))}
    </ul>
  )
}

interface TournamentsWidgetProps {
  isLoading: boolean
  isError: boolean
  tournaments: import("@/types/api/tournaments").Tournament[]
  orgSlug: string
  onRetry?: () => void
}

function TournamentsWidget({ isLoading, isError, tournaments, orgSlug, onRetry }: TournamentsWidgetProps) {
  if (isLoading) return <WidgetSkeleton rows={3} />
  if (isError) return <WidgetError onRetry={onRetry} />
  if (tournaments.length === 0) {
    return (
      <EmptyState
        icon={<TrophyIcon />}
        title="No active tournaments"
        description="Tournaments with open registration appear here."
        className="min-h-32 border-0 bg-transparent py-6"
      />
    )
  }
  return (
    <ul className="space-y-3" role="list">
      {tournaments.map((t) => (
        <li key={t.id} className="flex items-start justify-between gap-2">
          <div className="min-w-0 flex-1">
            <Link
              href={`/${orgSlug}/tournaments/${t.id}`}
              className="block truncate text-xs font-medium hover:underline"
            >
              {t.name}
            </Link>
            <p className="text-xs text-muted-foreground capitalize">{t.sport}</p>
          </div>
          <StatusBadge status={t.status} className="shrink-0 text-xs" />
        </li>
      ))}
    </ul>
  )
}

interface MatchesWidgetProps {
  isLoading: boolean
  isError: boolean
  matches: import("@/types/api/matches").Match[]
  onRetry?: () => void
}

function MatchesWidget({ isLoading, isError, matches, onRetry }: MatchesWidgetProps) {
  if (isLoading) return <WidgetSkeleton rows={3} />
  if (isError) return <WidgetError onRetry={onRetry} />
  if (matches.length === 0) {
    return (
      <EmptyState
        icon={<SwordsIcon />}
        title="No matches yet"
        description="Recent matches appear here."
        className="min-h-32 border-0 bg-transparent py-6"
      />
    )
  }
  return (
    <ul className="space-y-3" role="list">
      {matches.map((m) => (
        <li key={m.id} className="flex items-start justify-between gap-2">
          <div className="min-w-0 flex-1">
            <p className="truncate text-xs font-medium">
              {m.scheduled_at ? formatDateTime(m.scheduled_at) : "Unscheduled"}
            </p>
            {m.status === "live" && (
              <span className="flex items-center gap-1 text-xs font-medium text-[--color-success]">
                <ZapIcon className="size-3" /> Live
              </span>
            )}
            {m.status !== "live" && (
              <p className="text-xs text-muted-foreground">
                {m.home_score} – {m.away_score}
              </p>
            )}
          </div>
          <StatusBadge status={m.status} className="shrink-0 text-xs" />
        </li>
      ))}
    </ul>
  )
}

// ── Shared widget sub-components ──────────────────────────────────────────────

function WidgetSkeleton({ rows }: { rows: number }) {
  return (
    <div className="space-y-3" aria-busy="true" aria-label="Loading">
      {Array.from({ length: rows }).map((_, i) => (
        <div key={i} className="flex items-center gap-2">
          <Skeleton className="size-1.5 shrink-0 rounded-full" />
          <div className="flex-1 space-y-1">
            <Skeleton className="h-3 w-3/4" />
            <Skeleton className="h-2.5 w-1/2" />
          </div>
        </div>
      ))}
    </div>
  )
}

function WidgetError({ onRetry }: { onRetry?: () => void }) {
  return (
    <div className="flex min-h-20 flex-col items-center justify-center gap-1.5 text-center">
      <AlertCircleIcon className="size-5 text-muted-foreground" />
      <p className="text-xs text-muted-foreground">Failed to load</p>
      {onRetry && (
        <Button variant="outline" size="sm" onClick={onRetry} className="h-6 px-2 text-xs">
          Retry
        </Button>
      )}
    </div>
  )
}
