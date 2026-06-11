"use client"

import * as React from "react"
import { UserIcon, CameraIcon } from "lucide-react"
import { cn } from "@/lib/utils"
import { MediaUpload } from "@/components/ui/media-upload"
import { useMediaUpload } from "@/hooks/use-media-upload"
import { useQueryClient } from "@tanstack/react-query"
import { mediaKeys, playerKeys } from "@/lib/query-keys"

interface PlayerAvatarProps {
  orgSlug: string
  playerId: string
  avatarUrl?: string | null
  displayName: string
  size?: "sm" | "md" | "lg"
  canUpload?: boolean
}

const sizeClasses = {
  sm: "size-8 text-xs",
  md: "size-12 text-sm",
  lg: "size-20 text-xl",
}

export function PlayerAvatar({
  orgSlug,
  playerId,
  avatarUrl,
  displayName,
  size = "md",
  canUpload = false,
}: PlayerAvatarProps) {
  const [showUploader, setShowUploader] = React.useState(false)
  const { status, progress, upload, reset } = useMediaUpload(orgSlug, "player", playerId)
  const queryClient = useQueryClient()

  async function handleFile(file: File) {
    const result = await upload(file)
    if (result) {
      setShowUploader(false)
      queryClient.invalidateQueries({ queryKey: playerKeys.detail(orgSlug, playerId) })
      queryClient.invalidateQueries({ queryKey: mediaKeys.list(orgSlug, { entity_type: "player", entity_id: playerId }) })
    }
    reset()
  }

  const initials = displayName
    .split(" ")
    .map((w) => w[0])
    .slice(0, 2)
    .join("")
    .toUpperCase()

  if (canUpload && showUploader) {
    return (
      <div className="space-y-2">
        <MediaUpload
          status={status}
          progress={progress}
          onFileSelect={handleFile}
          onClear={() => { setShowUploader(false); reset() }}
          label="Upload avatar"
          className="w-48"
        />
      </div>
    )
  }

  return (
    <div className="relative inline-block">
      <div
        className={cn(
          "flex items-center justify-center rounded-full bg-muted font-medium text-muted-foreground",
          sizeClasses[size],
        )}
      >
        {avatarUrl ? (
          // eslint-disable-next-line @next/next/no-img-element
          <img
            src={avatarUrl}
            alt={displayName}
            className={cn("rounded-full object-cover", sizeClasses[size])}
          />
        ) : initials ? (
          <span>{initials}</span>
        ) : (
          <UserIcon className="size-1/2" />
        )}
      </div>
      {canUpload && (
        <button
          type="button"
          aria-label="Change avatar"
          onClick={() => setShowUploader(true)}
          className="absolute -bottom-1 -right-1 flex size-6 items-center justify-center rounded-full border-2 border-background bg-muted shadow-sm hover:bg-accent"
        >
          <CameraIcon className="size-3" />
        </button>
      )}
    </div>
  )
}

// ── Static display (no upload) ─────────────────────────────────────────────────

interface AvatarDisplayProps {
  avatarUrl?: string | null
  displayName: string
  size?: "sm" | "md" | "lg"
  className?: string
}

export function AvatarDisplay({ avatarUrl, displayName, size = "md", className }: AvatarDisplayProps) {
  const initials = displayName
    .split(" ")
    .map((w) => w[0])
    .slice(0, 2)
    .join("")
    .toUpperCase()

  return (
    <div
      className={cn(
        "flex shrink-0 items-center justify-center rounded-full bg-muted font-medium text-muted-foreground",
        sizeClasses[size],
        className,
      )}
    >
      {avatarUrl ? (
        // eslint-disable-next-line @next/next/no-img-element
        <img
          src={avatarUrl}
          alt={displayName}
          className={cn("rounded-full object-cover", sizeClasses[size])}
        />
      ) : initials ? (
        <span>{initials}</span>
      ) : (
        <UserIcon className="size-1/2" />
      )}
    </div>
  )
}
