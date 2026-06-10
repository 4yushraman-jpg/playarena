"use client"

import { Trash2Icon, CheckIcon } from "lucide-react"
import { Button } from "@/components/ui/button"
import { cn } from "@/lib/utils"
import { formatRelative } from "@/lib/format"
import type { Notification, NotificationEventType } from "@/types/api/notifications"

const EVENT_LABELS: Record<NotificationEventType, string> = {
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

function getEventDescription(notification: Notification): string {
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
      const status = (payload as { status?: string }).status
      return status ? `Tournament status changed to ${status}.` : "Tournament status has changed."
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

interface NotificationItemProps {
  notification: Notification
  onMarkRead: (id: string) => void
  onDelete: (id: string) => void
  isMarkingRead?: boolean
  isDeleting?: boolean
}

export function NotificationItem({
  notification,
  onMarkRead,
  onDelete,
  isMarkingRead,
  isDeleting,
}: NotificationItemProps) {
  const isUnread = notification.read_at === null

  return (
    <div
      className={cn(
        "group relative flex gap-3 rounded-lg border border-border p-4 transition-colors",
        isUnread
          ? "bg-accent/30 hover:bg-accent/50"
          : "bg-card hover:bg-muted/40",
        isDeleting && "opacity-50",
      )}
      role="article"
      aria-label={`${EVENT_LABELS[notification.event_type]}, ${isUnread ? "unread" : "read"}`}
    >
      {/* Unread dot */}
      <div className="mt-1 shrink-0">
        {isUnread ? (
          <span
            className="block size-2 rounded-full bg-primary"
            aria-hidden="true"
          />
        ) : (
          <span className="block size-2 rounded-full bg-transparent" aria-hidden="true" />
        )}
      </div>

      {/* Content */}
      <div className="min-w-0 flex-1 space-y-0.5">
        <p className={cn("text-sm", isUnread ? "font-medium" : "font-normal text-muted-foreground")}>
          {EVENT_LABELS[notification.event_type]}
        </p>
        <p className="text-xs text-muted-foreground">
          {getEventDescription(notification)}
        </p>
        <p className="text-xs text-muted-foreground/70">
          {formatRelative(notification.created_at)}
        </p>
      </div>

      {/* Actions */}
      <div className="flex shrink-0 items-start gap-1 opacity-0 transition-opacity group-hover:opacity-100 focus-within:opacity-100 [@media(hover:none)]:opacity-100">
        {isUnread && (
          <Button
            variant="ghost"
            size="icon-sm"
            onClick={() => onMarkRead(notification.id)}
            disabled={isMarkingRead}
            aria-label="Mark as read"
            title="Mark as read"
          >
            <CheckIcon />
          </Button>
        )}
        <Button
          variant="ghost"
          size="icon-sm"
          onClick={() => onDelete(notification.id)}
          disabled={isDeleting}
          aria-label="Delete notification"
          title="Delete notification"
          className="text-muted-foreground hover:text-destructive"
        >
          <Trash2Icon />
        </Button>
      </div>
    </div>
  )
}
