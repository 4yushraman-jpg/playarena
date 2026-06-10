"use client"

import { useParams } from "next/navigation"
import { useInfiniteQuery, useMutation, useQueryClient } from "@tanstack/react-query"
import { BellIcon, CheckCheckIcon, Loader2Icon } from "lucide-react"
import { toast } from "sonner"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { EmptyState } from "@/components/ui/empty-state"
import { PageHeader } from "@/components/ui/page-header"
import { NotificationItem } from "@/components/notifications/notification-item"
import { notificationKeys } from "@/lib/query-keys"
import { notificationsApi } from "@/lib/api/notifications"
import { useUnreadCount } from "@/hooks/use-unread-count"
import type { InfiniteData } from "@tanstack/react-query"
import type { Notification } from "@/types/api/notifications"

const PAGE_LIMIT = 20

interface NotifPage {
  notifications: Notification[]
  total: number
  limit: number
  offset: number
}

type NotifInfiniteData = InfiniteData<NotifPage, number>

const QUERY_KEY = (orgSlug: string) => notificationKeys.list(orgSlug, { limit: PAGE_LIMIT })

export default function NotificationsPage() {
  const params = useParams<{ orgSlug: string }>()
  const orgSlug = params.orgSlug
  const queryClient = useQueryClient()
  const { unreadCount } = useUnreadCount(orgSlug)

  const {
    data,
    isLoading,
    isError,
    refetch,
    fetchNextPage,
    hasNextPage,
    isFetchingNextPage,
  } = useInfiniteQuery({
    queryKey: QUERY_KEY(orgSlug),
    queryFn: ({ pageParam }) =>
      notificationsApi
        .list(orgSlug, { limit: PAGE_LIMIT, offset: pageParam })
        .then((r) => r.data),
    initialPageParam: 0,
    getNextPageParam: (lastPage, allPages) => {
      const loaded = allPages.reduce((sum, p) => sum + p.notifications.length, 0)
      return loaded < lastPage.total ? loaded : undefined
    },
    staleTime: 30_000,
  })

  const notifications = data?.pages.flatMap((p) => p.notifications) ?? []
  const total = data?.pages[0]?.total ?? 0

  // ── Mark single read ───────────────────────────────────────────────────────

  const markReadMutation = useMutation({
    mutationFn: (id: string) => notificationsApi.markRead(orgSlug, id),
    onMutate: async (id) => {
      await queryClient.cancelQueries({ queryKey: notificationKeys.all(orgSlug) })
      const prev = queryClient.getQueryData<NotifInfiniteData>(QUERY_KEY(orgSlug))
      queryClient.setQueryData<NotifInfiniteData>(QUERY_KEY(orgSlug), (old) => {
        if (!old) return old
        return {
          ...old,
          pages: old.pages.map((page) => ({
            ...page,
            notifications: page.notifications.map((n) =>
              n.id === id ? { ...n, read_at: new Date().toISOString() } : n,
            ),
          })),
        }
      })
      return { prev }
    },
    onError: (_err, _id, ctx) => {
      if (ctx?.prev) queryClient.setQueryData(QUERY_KEY(orgSlug), ctx.prev)
      toast.error("Failed to mark as read")
    },
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: notificationKeys.all(orgSlug) })
    },
  })

  // ── Mark all read ──────────────────────────────────────────────────────────

  const markAllReadMutation = useMutation({
    mutationFn: () => notificationsApi.markAllRead(orgSlug),
    onMutate: async () => {
      await queryClient.cancelQueries({ queryKey: notificationKeys.all(orgSlug) })
      const prev = queryClient.getQueryData<NotifInfiniteData>(QUERY_KEY(orgSlug))
      const now = new Date().toISOString()
      queryClient.setQueryData<NotifInfiniteData>(QUERY_KEY(orgSlug), (old) => {
        if (!old) return old
        return {
          ...old,
          pages: old.pages.map((page) => ({
            ...page,
            notifications: page.notifications.map((n) => ({
              ...n,
              read_at: n.read_at ?? now,
            })),
          })),
        }
      })
      return { prev }
    },
    onError: (_err, _vars, ctx) => {
      if (ctx?.prev) queryClient.setQueryData(QUERY_KEY(orgSlug), ctx.prev)
      toast.error("Failed to mark all as read")
    },
    onSuccess: () => {
      toast.success("All notifications marked as read")
    },
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: notificationKeys.all(orgSlug) })
    },
  })

  // ── Delete notification ────────────────────────────────────────────────────

  const deleteMutation = useMutation({
    mutationFn: (id: string) => notificationsApi.delete(orgSlug, id),
    onMutate: async (id) => {
      await queryClient.cancelQueries({ queryKey: notificationKeys.all(orgSlug) })
      const prev = queryClient.getQueryData<NotifInfiniteData>(QUERY_KEY(orgSlug))
      queryClient.setQueryData<NotifInfiniteData>(QUERY_KEY(orgSlug), (old) => {
        if (!old) return old
        return {
          ...old,
          pages: old.pages.map((page) => ({
            ...page,
            notifications: page.notifications.filter((n) => n.id !== id),
            total: Math.max(0, page.total - 1),
          })),
        }
      })
      return { prev }
    },
    onError: (_err, _id, ctx) => {
      if (ctx?.prev) queryClient.setQueryData(QUERY_KEY(orgSlug), ctx.prev)
      toast.error("Failed to delete notification")
    },
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: notificationKeys.all(orgSlug) })
    },
  })

  // ── Render ─────────────────────────────────────────────────────────────────

  return (
    <div className="space-y-6">
      <PageHeader
        title="Notifications"
        description={
          total > 0
            ? `${total} total · ${unreadCount} unread`
            : "All caught up"
        }
        action={
          unreadCount > 0 ? (
            <Button
              variant="outline"
              size="sm"
              onClick={() => markAllReadMutation.mutate()}
              disabled={markAllReadMutation.isPending}
              className="gap-2"
            >
              {markAllReadMutation.isPending ? (
                <Loader2Icon className="size-3.5 animate-spin" />
              ) : (
                <CheckCheckIcon className="size-3.5" />
              )}
              Mark all read
            </Button>
          ) : undefined
        }
      />

      {isLoading && <NotificationListSkeleton />}

      {isError && (
        <div className="flex flex-col items-center gap-3 rounded-xl border border-dashed border-border py-16 text-center">
          <p className="text-sm text-muted-foreground">Failed to load notifications.</p>
          <Button variant="outline" size="sm" onClick={() => refetch()}>
            Retry
          </Button>
        </div>
      )}

      {!isLoading && !isError && notifications.length === 0 && (
        <EmptyState
          icon={<BellIcon />}
          title="No notifications"
          description="New activity in your organization will appear here in real time."
        />
      )}

      {!isLoading && !isError && notifications.length > 0 && (
        <>
          <div
            className="space-y-2"
            role="feed"
            aria-label="Notification list"
            aria-busy={isLoading}
          >
            {notifications.map((n) => (
              <NotificationItem
                key={n.id}
                notification={n}
                onMarkRead={(id) => markReadMutation.mutate(id)}
                onDelete={(id) => deleteMutation.mutate(id)}
                isMarkingRead={
                  markReadMutation.isPending && markReadMutation.variables === n.id
                }
                isDeleting={
                  deleteMutation.isPending && deleteMutation.variables === n.id
                }
              />
            ))}
          </div>

          {hasNextPage && (
            <div className="flex justify-center pt-2">
              <Button
                variant="outline"
                size="sm"
                onClick={() => fetchNextPage()}
                disabled={isFetchingNextPage}
                className="gap-2"
              >
                {isFetchingNextPage && <Loader2Icon className="size-3.5 animate-spin" />}
                Load more
              </Button>
            </div>
          )}
        </>
      )}
    </div>
  )
}

function NotificationListSkeleton() {
  return (
    <div className="space-y-2" aria-busy="true" aria-label="Loading notifications">
      {Array.from({ length: 6 }).map((_, i) => (
        <div
          key={i}
          className="flex gap-3 rounded-lg border border-border p-4"
          style={{ opacity: 1 - i * 0.1 }}
        >
          <Skeleton className="mt-1 size-2 shrink-0 rounded-full" />
          <div className="flex-1 space-y-2">
            <Skeleton className="h-3.5 w-1/3" />
            <Skeleton className="h-3 w-2/3" />
            <Skeleton className="h-2.5 w-1/4" />
          </div>
        </div>
      ))}
    </div>
  )
}
