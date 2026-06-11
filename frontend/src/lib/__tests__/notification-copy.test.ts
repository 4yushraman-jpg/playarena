import { describe, it, expect } from "vitest"
import {
  NOTIFICATION_EVENT_LABELS,
  getNotificationDescription,
  getNotificationLabel,
} from "../notification-copy"
import type { NotificationEventType } from "@/types/api/notifications"

const ALL_EVENT_TYPES: NotificationEventType[] = [
  "match_created",
  "match_started",
  "match_completed",
  "match_cancelled",
  "match_abandoned",
  "tournament_status_changed",
  "registration_approved",
  "registration_rejected",
  "registration_withdrawn",
]

describe("notification copy mapping", () => {
  it("has human copy for every event type — no raw event names leak to the UI", () => {
    for (const eventType of ALL_EVENT_TYPES) {
      const label = getNotificationLabel(eventType)
      expect(label).toBe(NOTIFICATION_EVENT_LABELS[eventType])
      // A raw event_type contains an underscore; human copy must not.
      expect(label).not.toMatch(/_/)
      expect(label.charAt(0)).toBe(label.charAt(0).toUpperCase())
    }
  })

  it("provides a description for every event type", () => {
    for (const eventType of ALL_EVENT_TYPES) {
      const description = getNotificationDescription({ event_type: eventType, payload: {} })
      expect(description.length).toBeGreaterThan(10)
      expect(description).not.toMatch(/_/)
    }
  })

  it("includes the new status in tournament_status_changed descriptions", () => {
    expect(
      getNotificationDescription({
        event_type: "tournament_status_changed",
        payload: { new_status: "registration_open" },
      }),
    ).toBe("Tournament status changed to registration open.")
  })
})
