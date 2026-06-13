"use client"

import { Loader2Icon, CheckCircleIcon, WifiOffIcon, AlertTriangleIcon } from "lucide-react"
import { cn } from "@/lib/utils"

interface SyncStatusProps {
  isOnline: boolean
  isSyncing: boolean
  unsyncedCount: number
  hasFailed: boolean
}

/**
 * Live sync indicator for scoring mode — always visible so the scorer can trust
 * what's recorded. Priority: rejected events (needs attention) → offline →
 * syncing → all synced. Every scored point is reflected in `unsyncedCount`
 * until the authoritative log confirms it.
 */
export function SyncStatus({ isOnline, isSyncing, unsyncedCount, hasFailed }: SyncStatusProps) {
  const { icon, text, className } = resolve({ isOnline, isSyncing, unsyncedCount, hasFailed })
  return (
    <div
      role="status"
      aria-live="polite"
      className={cn(
        "flex items-center gap-2 rounded-lg border px-3 py-1.5 text-xs font-medium",
        className,
      )}
    >
      {icon}
      <span className="truncate">{text}</span>
    </div>
  )
}

function resolve({ isOnline, isSyncing, unsyncedCount, hasFailed }: SyncStatusProps) {
  if (hasFailed) {
    return {
      icon: <AlertTriangleIcon className="size-3.5 shrink-0" />,
      text: "Some events were rejected — needs attention",
      className:
        "border-red-300 bg-red-50 text-red-800 dark:border-red-900 dark:bg-red-950/30 dark:text-red-200",
    }
  }
  if (!isOnline) {
    return {
      icon: <WifiOffIcon className="size-3.5 shrink-0" />,
      text:
        unsyncedCount > 0
          ? `Offline · ${unsyncedCount} queued — will sync on reconnect`
          : "Offline",
      className:
        "border-amber-300 bg-amber-50 text-amber-800 dark:border-amber-900 dark:bg-amber-950/30 dark:text-amber-200",
    }
  }
  if (unsyncedCount > 0 || isSyncing) {
    return {
      icon: <Loader2Icon className="size-3.5 shrink-0 animate-spin" />,
      text:
        unsyncedCount > 0
          ? `Syncing ${unsyncedCount} event${unsyncedCount === 1 ? "" : "s"}…`
          : "Syncing…",
      className: "border-border bg-muted/50 text-muted-foreground",
    }
  }
  return {
    icon: <CheckCircleIcon className="size-3.5 shrink-0 text-green-600 dark:text-green-400" />,
    text: "All events synced",
    className: "border-border bg-muted/40 text-muted-foreground",
  }
}
