"use client"

import { useState, useCallback } from "react"
import { useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"
import { mediaApi } from "@/lib/api/media"
import { mediaKeys } from "@/lib/query-keys"
import { extractApiError } from "@/lib/api-error"
import type { MediaEntityType, MediaAttachment } from "@/types/api/media"

type UploadStatus = "idle" | "uploading" | "success" | "error"

export interface UseMediaUploadReturn {
  status: UploadStatus
  progress: number
  upload: (file: File) => Promise<MediaAttachment | null>
  reset: () => void
}

export function useMediaUpload(
  orgSlug: string,
  entityType: MediaEntityType,
  entityId: string,
): UseMediaUploadReturn {
  const queryClient = useQueryClient()
  const [status, setStatus] = useState<UploadStatus>("idle")
  const [progress, setProgress] = useState(0)

  const upload = useCallback(
    async (file: File): Promise<MediaAttachment | null> => {
      setStatus("uploading")
      setProgress(0)
      try {
        const response = await mediaApi.upload(
          orgSlug,
          file,
          entityType,
          entityId,
          setProgress,
        )
        setStatus("success")
        queryClient.invalidateQueries({
          queryKey: mediaKeys.list(orgSlug, { entity_type: entityType, entity_id: entityId }),
        })
        toast.success("Upload complete")
        return response.data
      } catch (err) {
        setStatus("error")
        toast.error(extractApiError(err))
        return null
      }
    },
    [orgSlug, entityType, entityId, queryClient],
  )

  const reset = useCallback(() => {
    setStatus("idle")
    setProgress(0)
  }, [])

  return { status, progress, upload, reset }
}
