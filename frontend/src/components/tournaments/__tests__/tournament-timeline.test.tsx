import { describe, it, expect } from "vitest"
import { render, screen } from "@testing-library/react"
import {
  TournamentTimeline,
  getTimelineSummary,
  TIMELINE_STEPS,
} from "../tournament-timeline"

describe("getTimelineSummary — compact mobile copy", () => {
  it("describes the current position in the lifecycle", () => {
    expect(getTimelineSummary("draft")).toBe("Step 1 of 5 · Draft")
    expect(getTimelineSummary("registration_open")).toBe("Step 2 of 5 · Open")
    expect(getTimelineSummary("completed")).toBe("Step 5 of 5 · Completed")
  })

  it("returns null for cancelled (rendered as a banner instead)", () => {
    expect(getTimelineSummary("cancelled")).toBeNull()
  })
})

describe("TournamentTimeline — semantics", () => {
  it("marks exactly the current step with aria-current on the list item", () => {
    render(<TournamentTimeline status="registration_closed" />)
    // Both the compact and full variants mark their li; each variant exactly once.
    const current = document.querySelectorAll('li[aria-current="step"]')
    expect(current.length).toBe(2)
    for (const li of current) {
      expect(li.tagName).toBe("LI")
      expect(li.textContent).toContain("Closed")
    }
  })

  it("renders every lifecycle step in the rail", () => {
    render(<TournamentTimeline status="draft" />)
    for (const step of TIMELINE_STEPS) {
      expect(screen.getAllByText(step.label).length).toBeGreaterThan(0)
    }
  })

  it("renders the compact summary for narrow viewports", () => {
    render(<TournamentTimeline status="ongoing" />)
    expect(screen.getByText("Step 4 of 5 · In Progress")).toBeInTheDocument()
  })

  it("renders a status banner instead of the rail when cancelled", () => {
    render(<TournamentTimeline status="cancelled" />)
    expect(screen.getByRole("status")).toHaveTextContent("This tournament was cancelled.")
    expect(document.querySelector('li[aria-current="step"]')).toBeNull()
  })
})
