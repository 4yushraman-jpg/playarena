"use client"

import { useQuery } from "@tanstack/react-query"
import { userKeys } from "@/lib/query-keys"
import { usersApi } from "@/lib/api/users"
import { useAuthStore, selectUserId } from "@/stores/auth.store"

export function useCurrentUser() {
  const userId = useAuthStore(selectUserId)

  return useQuery({
    queryKey: userKeys.detail(userId ?? ""),
    queryFn: () => usersApi.get(userId!).then((r) => r.data),
    enabled: !!userId,
    staleTime: 5 * 60_000,
  })
}
