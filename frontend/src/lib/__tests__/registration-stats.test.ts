import { describe, it, expect } from "vitest"
import {
  EMPTY_REGISTRATION_COUNTS,
  formatCapacityLabel,
  getCapacityUsage,
  getParticipantLabel,
  getRegistrationCounts,
} from "../registration-stats"
import type { RegistrationCounts } from "@/types/api/tournaments"

function counts(overrides: Partial<RegistrationCounts> = {}): RegistrationCounts {
  return { ...EMPTY_REGISTRATION_COUNTS, ...overrides }
}

describe("getRegistrationCounts", () => {
  it("returns server counts when present", () => {
    const c = counts({ pending: 2, approved: 3, active: 5, total: 5 })
    expect(getRegistrationCounts({ registration_counts: c })).toEqual(c)
  })

  it("falls back to zeros when the response predates counts", () => {
    expect(getRegistrationCounts({})).toEqual(EMPTY_REGISTRATION_COUNTS)
  })
})

describe("getCapacityUsage — capacity bar correctness", () => {
  it("uses the ACTIVE count (pending + approved), not approved-only", () => {
    // Backend enforces capacity against pending+approved. 6 approved + 10
    // pending against max 16 means the tournament is FULL even though the
    // approved-only view would claim 37% usage.
    const usage = getCapacityUsage({
      max_participants: 16,
      registration_counts: counts({ approved: 6, pending: 10, active: 16, total: 16 }),
    })
    expect(usage).toEqual({ used: 16, max: 16, pct: 100, isFull: true })
  })

  it("returns null when the tournament has no cap", () => {
    expect(
      getCapacityUsage({ max_participants: null, registration_counts: counts() }),
    ).toBeNull()
  })

  it("clamps percentage at 100 even if active exceeds max", () => {
    const usage = getCapacityUsage({
      max_participants: 8,
      registration_counts: counts({ approved: 9, active: 9, total: 9 }),
    })
    expect(usage?.pct).toBe(100)
    expect(usage?.isFull).toBe(true)
  })

  it("is not full one below the cap", () => {
    const usage = getCapacityUsage({
      max_participants: 8,
      registration_counts: counts({ approved: 4, pending: 3, active: 7, total: 7 }),
    })
    expect(usage?.isFull).toBe(false)
    expect(usage?.used).toBe(7)
  })
})

describe("formatCapacityLabel — directory capacity column", () => {
  it("shows used/max for capped tournaments", () => {
    expect(
      formatCapacityLabel({
        max_participants: 16,
        registration_counts: counts({ approved: 3, pending: 5, active: 8, total: 8 }),
      }),
    ).toBe("8 / 16")
  })

  it("shows a registered count for uncapped tournaments instead of a dash", () => {
    expect(
      formatCapacityLabel({
        max_participants: null,
        registration_counts: counts({ approved: 5, active: 5, total: 5 }),
      }),
    ).toBe("5 registered")
    expect(
      formatCapacityLabel({
        max_participants: null,
        registration_counts: counts({ approved: 1, active: 1, total: 1 }),
      }),
    ).toBe("1 registered")
  })

  it("shows zero usage rather than a placeholder when there are no registrations", () => {
    expect(formatCapacityLabel({ max_participants: 16, registration_counts: counts() })).toBe(
      "0 / 16",
    )
  })
})

describe("getParticipantLabel — team name rendering", () => {
  it("prefers the joined team name", () => {
    expect(
      getParticipantLabel({
        team_id: "3f2a1b9c-0000-0000-0000-000000000000",
        player_id: null,
        team_name: "Thunder Strikers",
        player_name: null,
      }),
    ).toBe("Thunder Strikers")
  })

  it("prefers the joined player name for individual registrations", () => {
    expect(
      getParticipantLabel({
        team_id: null,
        player_id: "abc12345-0000-0000-0000-000000000000",
        team_name: null,
        player_name: "Asha Rao",
      }),
    ).toBe("Asha Rao")
  })

  it("falls back to a shortened id only when no name is available", () => {
    expect(
      getParticipantLabel({
        team_id: "3f2a1b9c-0000-0000-0000-000000000000",
        player_id: null,
      }),
    ).toBe("Team 3f2a1b9c")
  })

  it("handles registrations with no participant reference", () => {
    expect(
      getParticipantLabel({ team_id: null, player_id: null }),
    ).toBe("Unknown participant")
  })
})
