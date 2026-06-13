"use client"

import { RefreshCwIcon, CheckCircleIcon, WifiOffIcon } from "lucide-react"
import { Button } from "@/components/ui/button"
import { cn } from "@/lib/utils"
import { formatRelative } from "@/lib/format"

interface SyncBannerProps {
  updatedAt: number | null
  isFetching: boolean
  isError: boolean
  onRefresh: () => void
}

/**
 * Read-only sync/freshness indicator. FE-7BA has no write queue, so "sync"
 * means "how current is the score you're looking at". The headline score is the
 * authoritative server value; this shows when it was last fetched, signals a
 * failed refresh, and offers a manual retry. (The offline queue + live sync
 * state arrive in FE-7BB.)
 */
export function SyncBanner({ updatedAt, isFetching, isError, onRefresh }: SyncBannerProps) {
  return (
    <div
      className={cn(
        "flex items-center justify-between gap-2 rounded-lg border px-3 py-2 text-xs",
        isError
          ? "border-amber-200 bg-amber-50 dark:border-amber-900 dark:bg-amber-950/30"
          : "border-border bg-muted/40",
      )}
    >
      <span className="flex min-w-0 items-center gap-1.5 text-muted-foreground">
        {isError ? (
          <>
            <WifiOffIcon className="size-3.5 shrink-0 text-amber-600 dark:text-amber-400" />
            <span className="truncate text-amber-800 dark:text-amber-200">
              Couldn&apos;t reach the server — showing the last known score
            </span>
          </>
        ) : (
          <>
            <CheckCircleIcon className="size-3.5 shrink-0 text-green-600 dark:text-green-400" />
            <span className="truncate">
              Score from server
              {updatedAt ? ` · updated ${formatRelative(new Date(updatedAt).toISOString())}` : ""}
            </span>
          </>
        )}
      </span>
      <Button
        variant="ghost"
        size="sm"
        className="h-9 shrink-0 gap-1.5 px-3"
        onClick={onRefresh}
        disabled={isFetching}
        aria-label="Refresh score"
      >
        <RefreshCwIcon className={isFetching ? "size-3.5 animate-spin" : "size-3.5"} />
        Refresh
      </Button>
    </div>
  )
}
