"use client"

import { useMutation, useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"
import { PlayIcon, Loader2Icon } from "lucide-react"
import { matchesApi } from "@/lib/api/matches"
import { matchKeys } from "@/lib/query-keys"
import { extractApiError } from "@/lib/api-error"

interface StartMatchButtonProps {
  orgSlug: string
  matchId: string
}

/**
 * Transitions a scheduled match to live (PATCH status=live), which is the
 * precondition for scoring (the backend rejects event writes on a non-live
 * match). Gated by the caller on match.update permission.
 */
export function StartMatchButton({ orgSlug, matchId }: StartMatchButtonProps) {
  const queryClient = useQueryClient()
  const startMatch = useMutation({
    mutationFn: () => matchesApi.update(orgSlug, matchId, { status: "live" }),
    onSuccess: (res) => {
      queryClient.setQueryData(matchKeys.detail(orgSlug, matchId), res.data)
      queryClient.invalidateQueries({ queryKey: matchKeys.score(orgSlug, matchId) })
      queryClient.invalidateQueries({ queryKey: matchKeys.eventsRoot(orgSlug, matchId) })
      toast.success("Match started")
    },
    onError: (err) => toast.error(extractApiError(err)),
  })

  return (
    <button
      type="button"
      onClick={() => startMatch.mutate()}
      disabled={startMatch.isPending}
      className="flex min-h-14 w-full items-center justify-center gap-2 rounded-xl bg-primary px-4 text-base font-semibold text-primary-foreground transition-colors hover:bg-primary/90 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:opacity-60"
    >
      {startMatch.isPending ? (
        <Loader2Icon className="size-5 animate-spin" />
      ) : (
        <PlayIcon className="size-5" />
      )}
      Start match
    </button>
  )
}
