import type { Notification, NotificationEventType } from "@/types/api/notifications"

/**
 * Human-readable copy for notification events. Single source of truth shared
 * by the notification center and dashboard widgets — never render a raw
 * event_type string in the UI.
 */
export const NOTIFICATION_EVENT_LABELS: Record<NotificationEventType, string> = {
  match_created: "Match Created",
  match_started: "Match Started",
  match_completed: "Match Completed",
  match_cancelled: "Match Cancelled",
  match_abandoned: "Match Abandoned",
  tournament_status_changed: "Tournament Updated",
  registration_approved: "Registration Approved",
  registration_rejected: "Registration Rejected",
  registration_withdrawn: "Registration Withdrawn",
}

export function getNotificationLabel(eventType: NotificationEventType): string {
  return NOTIFICATION_EVENT_LABELS[eventType] ?? "Notification"
}

export function getNotificationDescription(
  notification: Pick<Notification, "event_type" | "payload">,
): string {
  const payload = notification.payload
  switch (notification.event_type) {
    case "match_started":
      return "A match has gone live. View the live score."
    case "match_completed":
      return "Match has ended. Check the final result."
    case "match_cancelled":
      return "A scheduled match has been cancelled."
    case "match_abandoned":
      return "A match was abandoned before completion."
    case "match_created":
      return "A new match has been scheduled."
    case "tournament_status_changed": {
      const status = (payload as { new_status?: string; status?: string }).new_status
        ?? (payload as { status?: string }).status
      return status
        ? `Tournament status changed to ${status.replace(/_/g, " ")}.`
        : "Tournament status has changed."
    }
    case "registration_approved":
      return "Your tournament registration has been approved."
    case "registration_rejected":
      return "Your tournament registration was not approved."
    case "registration_withdrawn":
      return "A registration has been withdrawn."
    default:
      return "You have a new notification."
  }
}
