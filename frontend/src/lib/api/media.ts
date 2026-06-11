"use client"

import api from "./client"
import type {
  MediaAttachment,
  MediaListParams,
  MediaListResponse,
  UpdateMediaRequest,
  MediaEntityType,
} from "@/types/api/media"

export const mediaApi = {
  list: (orgSlug: string, params?: MediaListParams) =>
    api.get<MediaListResponse>(
      `/api/v1/organizations/${orgSlug}/media`,
      { params },
    ),

  getById: (orgSlug: string, id: string) =>
    api.get<MediaAttachment>(`/api/v1/organizations/${orgSlug}/media/${id}`),

  upload: (
    orgSlug: string,
    file: File,
    entityType: MediaEntityType,
    entityId: string,
    onProgress?: (percent: number) => void,
  ) => {
    const form = new FormData()
    form.append("file", file)
    form.append("entity_type", entityType)
    form.append("entity_id", entityId)
    return api.post<MediaAttachment>(
      `/api/v1/organizations/${orgSlug}/media`,
      form,
      {
        // Do NOT set Content-Type manually — the browser sets
        // multipart/form-data with the correct boundary automatically.
        onUploadProgress: (event) => {
          if (onProgress && event.total) {
            onProgress(Math.round((event.loaded / event.total) * 100))
          }
        },
      },
    )
  },

  update: (orgSlug: string, id: string, body: UpdateMediaRequest) =>
    api.patch<MediaAttachment>(`/api/v1/organizations/${orgSlug}/media/${id}`, body),

  delete: (orgSlug: string, id: string) =>
    api.delete(`/api/v1/organizations/${orgSlug}/media/${id}`),
}
