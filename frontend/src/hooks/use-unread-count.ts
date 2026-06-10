"use client"

import { useQuery } from "@tanstack/react-query"
import { notificationKeys } from "@/lib/query-keys"
import { notificationsApi } from "@/lib/api/notifications"

export function useUnreadCount(orgSlug: string) {
  const { data } = useQuery({
    queryKey: notificationKeys.list(orgSlug, { limit: 50, offset: 0 }),
    queryFn: () =>
      notificationsApi.list(orgSlug, { limit: 50, offset: 0 }).then((r) => r.data),
    staleTime: 30_000,
  })

  const notifications = data?.notifications ?? []
  const unreadCount = notifications.filter((n) => n.read_at === null).length

  return { unreadCount }
}
