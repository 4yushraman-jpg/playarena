export type NotificationEventType =
  | "match_created"
  | "match_started"
  | "match_completed"
  | "match_cancelled"
  | "match_abandoned"
  | "tournament_status_changed"
  | "registration_approved"
  | "registration_rejected"
  | "registration_withdrawn"

export type NotificationChannel = "in_app" | "email" | "webhook"

export interface Notification {
  id: string
  organization_id: string
  user_id: string
  outbox_id: string
  channel: NotificationChannel
  event_type: NotificationEventType
  entity_type: string
  entity_id: string
  payload: Record<string, unknown>
  read_at: string | null
  sent_at: string | null
  created_at: string
}

export interface NotificationPreference {
  id: string
  organization_id: string
  user_id: string
  event_type: NotificationEventType
  channel: NotificationChannel
  enabled: boolean
  updated_at: string
}

export interface UpdatePreferenceRequest {
  channel: NotificationChannel
  enabled: boolean
}

export interface NotificationListParams {
  limit?: number
  offset?: number
}

// SSE stream event payload
export interface NotificationStreamEvent {
  id: string
  event_type: NotificationEventType
  entity_type: string
  entity_id: string
  payload: Record<string, unknown>
  created_at: string
}
