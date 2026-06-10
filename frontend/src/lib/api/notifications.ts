"use client"

import api from "./client"
import type {
  Notification,
  NotificationPreference,
  NotificationListParams,
  UpdatePreferenceRequest,
  NotificationEventType,
} from "@/types/api/notifications"

interface NotificationListResponse {
  notifications: Notification[]
  total: number
  limit: number
  offset: number
}

interface PreferencesResponse {
  preferences: NotificationPreference[]
}

const base = (orgSlug: string) => `/api/v1/organizations/${orgSlug}/notifications`

export const notificationsApi = {
  list: (orgSlug: string, params?: NotificationListParams) =>
    api.get<NotificationListResponse>(base(orgSlug), { params }),

  markRead: (orgSlug: string, id: string) =>
    api.post<{ message: string }>(`${base(orgSlug)}/${id}/read`),

  markAllRead: (orgSlug: string) =>
    api.post<{ message: string }>(`${base(orgSlug)}/read-all`),

  delete: (orgSlug: string, id: string) =>
    api.delete(`${base(orgSlug)}/${id}`),

  getPreferences: (orgSlug: string) =>
    api.get<PreferencesResponse>(`${base(orgSlug)}/preferences`),

  updatePreference: (
    orgSlug: string,
    eventType: NotificationEventType,
    data: UpdatePreferenceRequest,
  ) =>
    api.patch<NotificationPreference>(
      `${base(orgSlug)}/preferences/${eventType}`,
      data,
    ),
}
