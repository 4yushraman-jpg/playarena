// Backend media.Response entity_type values
export type MediaEntityType =
  | "organization"
  | "player"
  | "team"
  | "tournament"
  | "match"
  | "user"

// Backend media.Response — matches backend exactly
export interface MediaAttachment {
  id: string
  entity_type: MediaEntityType
  entity_id: string
  media_type: string          // e.g. "image", "video"
  file_name: string
  file_size: number | null
  mime_type: string | null
  width: number | null
  height: number | null
  alt_text: string | null
  is_primary: boolean
  sort_order: number
  file_url: string            // CDN-ready URL
  uploaded_by: string | null
  created_at: string
  updated_at: string
}

export interface MediaListResponse {
  attachments: MediaAttachment[]
  total: number
  limit: number
  offset: number
}

// PATCH body — all fields optional
export interface UpdateMediaRequest {
  alt_text?: string | null
  sort_order?: number
  is_primary?: boolean
}

export interface MediaListParams {
  entity_type?: MediaEntityType
  entity_id?: string
  limit?: number
  offset?: number
}
