import { describe, it, expect, vi } from "vitest"
import { screen } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { renderWithProviders } from "@/test/test-utils"
import { NotificationItem } from "../notification-item"
import type { Notification } from "@/types/api/notifications"

function makeNotification(overrides: Partial<Notification> = {}): Notification {
  return {
    id: "n1",
    organization_id: "org1",
    user_id: "u1",
    outbox_id: "o1",
    channel: "in_app",
    event_type: "match_started",
    entity_type: "match",
    entity_id: "m1",
    payload: {},
    read_at: null,
    sent_at: null,
    created_at: new Date().toISOString(),
    ...overrides,
  }
}

describe("NotificationItem", () => {
  it("renders event label and relative time", () => {
    renderWithProviders(
      <NotificationItem
        notification={makeNotification()}
        onMarkRead={vi.fn()}
        onDelete={vi.fn()}
      />,
    )
    expect(screen.getByText("Match Started")).toBeInTheDocument()
  })

  it("shows 'Mark as read' button only for unread notifications", () => {
    renderWithProviders(
      <NotificationItem
        notification={makeNotification({ read_at: null })}
        onMarkRead={vi.fn()}
        onDelete={vi.fn()}
      />,
    )
    expect(screen.getByRole("button", { name: /mark as read/i })).toBeInTheDocument()
  })

  it("hides 'Mark as read' button for already-read notifications", () => {
    renderWithProviders(
      <NotificationItem
        notification={makeNotification({ read_at: "2024-01-01T00:00:00Z" })}
        onMarkRead={vi.fn()}
        onDelete={vi.fn()}
      />,
    )
    expect(screen.queryByRole("button", { name: /mark as read/i })).toBeNull()
  })

  it("always renders the delete button", () => {
    renderWithProviders(
      <NotificationItem
        notification={makeNotification()}
        onMarkRead={vi.fn()}
        onDelete={vi.fn()}
      />,
    )
    expect(screen.getByRole("button", { name: /delete notification/i })).toBeInTheDocument()
  })

  it("action buttons are in the DOM and reachable on touch devices without hover", () => {
    const { container } = renderWithProviders(
      <NotificationItem
        notification={makeNotification({ read_at: null })}
        onMarkRead={vi.fn()}
        onDelete={vi.fn()}
      />,
    )
    // Buttons must always be in DOM regardless of hover state
    expect(screen.getByRole("button", { name: /mark as read/i })).toBeInTheDocument()
    expect(screen.getByRole("button", { name: /delete notification/i })).toBeInTheDocument()

    // The actions container must include the touch media query class so
    // actions are visible on touch devices (no hover capability)
    const deleteBtn = screen.getByRole("button", { name: /delete notification/i })
    const actionsDiv = deleteBtn.parentElement!
    expect(actionsDiv.className).toContain("[@media(hover:none)]:opacity-100")
    void container  // suppress unused-variable lint
  })

  it("calls onMarkRead with the notification id when clicked", async () => {
    const user = userEvent.setup()
    const onMarkRead = vi.fn()
    renderWithProviders(
      <NotificationItem
        notification={makeNotification({ read_at: null })}
        onMarkRead={onMarkRead}
        onDelete={vi.fn()}
      />,
    )
    await user.click(screen.getByRole("button", { name: /mark as read/i }))
    expect(onMarkRead).toHaveBeenCalledWith("n1")
  })

  it("calls onDelete with the notification id when clicked", async () => {
    const user = userEvent.setup()
    const onDelete = vi.fn()
    renderWithProviders(
      <NotificationItem
        notification={makeNotification({ read_at: null })}
        onMarkRead={vi.fn()}
        onDelete={onDelete}
      />,
    )
    await user.click(screen.getByRole("button", { name: /delete notification/i }))
    expect(onDelete).toHaveBeenCalledWith("n1")
  })
})
