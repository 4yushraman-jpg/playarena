"use client"

import * as React from "react"
import { ShieldIcon, CameraIcon } from "lucide-react"
import { cn } from "@/lib/utils"
import { MediaUpload } from "@/components/ui/media-upload"
import { useMediaUpload } from "@/hooks/use-media-upload"
import { useQueryClient } from "@tanstack/react-query"
import { mediaKeys, teamKeys } from "@/lib/query-keys"

interface TeamLogoProps {
  orgSlug: string
  teamId: string
  logoUrl?: string | null
  teamName: string
  primaryColor?: string | null
  size?: "sm" | "md" | "lg"
  canUpload?: boolean
}

const sizeClasses = {
  sm: "size-8 text-xs",
  md: "size-12 text-sm",
  lg: "size-20 text-xl",
}

export function TeamLogo({
  orgSlug,
  teamId,
  logoUrl,
  teamName,
  primaryColor,
  size = "md",
  canUpload = false,
}: TeamLogoProps) {
  const [showUploader, setShowUploader] = React.useState(false)
  const { status, progress, upload, reset } = useMediaUpload(orgSlug, "team", teamId)
  const queryClient = useQueryClient()

  async function handleFile(file: File) {
    const result = await upload(file)
    if (result) {
      setShowUploader(false)
      queryClient.invalidateQueries({ queryKey: teamKeys.detail(orgSlug, teamId) })
      queryClient.invalidateQueries({ queryKey: mediaKeys.list(orgSlug, { entity_type: "team", entity_id: teamId }) })
    }
    reset()
  }

  const initials = teamName
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
          label="Upload team logo"
          className="w-48"
        />
      </div>
    )
  }

  return (
    <div className="relative inline-block">
      <div
        className={cn(
          "flex items-center justify-center rounded-xl font-bold bg-muted text-muted-foreground",
          sizeClasses[size],
        )}
        style={logoUrl ? undefined : (primaryColor ? { backgroundColor: primaryColor, color: "white" } : undefined)}
      >
        {logoUrl ? (
          // eslint-disable-next-line @next/next/no-img-element
          <img
            src={logoUrl}
            alt={teamName}
            className={cn("rounded-xl object-cover", sizeClasses[size])}
          />
        ) : (
          initials || <ShieldIcon className="size-1/2" />
        )}
      </div>
      {canUpload && (
        <button
          type="button"
          aria-label="Change team logo"
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

interface LogoDisplayProps {
  logoUrl?: string | null
  teamName: string
  primaryColor?: string | null
  size?: "sm" | "md" | "lg"
  className?: string
}

export function LogoDisplay({ logoUrl, teamName, primaryColor, size = "md", className }: LogoDisplayProps) {
  const initials = teamName
    .split(" ")
    .map((w) => w[0])
    .slice(0, 2)
    .join("")
    .toUpperCase()

  return (
    <div
      className={cn(
        "flex shrink-0 items-center justify-center rounded-xl bg-muted font-bold text-muted-foreground",
        sizeClasses[size],
        className,
      )}
      style={logoUrl ? undefined : (primaryColor ? { backgroundColor: primaryColor, color: "white" } : undefined)}
    >
      {logoUrl ? (
        // eslint-disable-next-line @next/next/no-img-element
        <img
          src={logoUrl}
          alt={teamName}
          className={cn("rounded-xl object-cover", sizeClasses[size])}
        />
      ) : (
        initials || <ShieldIcon className="size-1/2" />
      )}
    </div>
  )
}
