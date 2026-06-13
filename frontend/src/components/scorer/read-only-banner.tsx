"use client"

import { EyeIcon, LockIcon } from "lucide-react"
import { cn } from "@/lib/utils"
import type { MatchStatus } from "@/types/api/matches"

interface ReadOnlyBannerProps {
  status: MatchStatus
}

/**
 * Communicates that this surface is read-only and why. Scoring controls are
 * introduced in FE-7BB; until then every match — live or terminal — is a
 * read-only scoreboard. Terminal matches additionally convey finality.
 */
export function ReadOnlyBanner({ status }: ReadOnlyBannerProps) {
  const config = bannerConfig(status)
  return (
    <div
      role="status"
      className={cn(
        "flex items-start gap-2.5 rounded-lg border px-3 py-2.5 text-sm",
        config.className,
      )}
    >
      {config.icon}
      <div className="min-w-0">
        <p className="font-medium">{config.title}</p>
        <p className="text-xs opacity-80">{config.body}</p>
      </div>
    </div>
  )
}

function bannerConfig(status: MatchStatus) {
  switch (status) {
    case "live":
      return {
        title: "Live — read-only scoreboard",
        body: "You're viewing the authoritative score and event history.",
        icon: <EyeIcon className="mt-0.5 size-4 shrink-0" />,
        className:
          "border-green-200 bg-green-50 text-green-800 dark:border-green-900 dark:bg-green-950/30 dark:text-green-200",
      }
    case "completed":
      return {
        title: "Final result",
        body: "This match is complete. The result is read-only.",
        icon: <LockIcon className="mt-0.5 size-4 shrink-0" />,
        className: "border-border bg-muted/50 text-foreground",
      }
    case "cancelled":
      return {
        title: "Match cancelled",
        body: "This fixture was cancelled and was not played.",
        icon: <LockIcon className="mt-0.5 size-4 shrink-0" />,
        className:
          "border-red-200 bg-red-50 text-red-800 dark:border-red-900 dark:bg-red-950/30 dark:text-red-200",
      }
    case "abandoned":
      return {
        title: "Match abandoned",
        body: "This match was abandoned before a result was recorded.",
        icon: <LockIcon className="mt-0.5 size-4 shrink-0" />,
        className:
          "border-orange-200 bg-orange-50 text-orange-800 dark:border-orange-900 dark:bg-orange-950/30 dark:text-orange-200",
      }
    default:
      return {
        title: "Read-only",
        body: "Viewing match details.",
        icon: <EyeIcon className="mt-0.5 size-4 shrink-0" />,
        className: "border-border bg-muted/50 text-foreground",
      }
  }
}
