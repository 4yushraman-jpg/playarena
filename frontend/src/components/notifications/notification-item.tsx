"use client"

import { Trash2Icon, CheckIcon } from "lucide-react"
import { Button } from "@/components/ui/button"
import { cn } from "@/lib/utils"
import { formatRelative } from "@/lib/format"
import {
  getNotificationDescription,
  getNotificationLabel,
} from "@/lib/notification-copy"
import type { Notification } from "@/types/api/notifications"

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
      aria-label={`${getNotificationLabel(notification.event_type)}, ${isUnread ? "unread" : "read"}`}
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
          {getNotificationLabel(notification.event_type)}
        </p>
        <p className="text-xs text-muted-foreground">
          {getNotificationDescription(notification)}
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
