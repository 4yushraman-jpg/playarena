import { describe, it, expect } from "vitest"
import {
  analyzeEligibility,
  getPickerOverflowHint,
} from "../register-participant-dialog"
import { EMPTY_REGISTRATION_COUNTS } from "@/lib/registration-stats"
import type { Tournament, RegistrationCounts } from "@/types/api/tournaments"
import type { TournamentRegistration } from "@/types/api/tournament-registrations"

function makeTournament(overrides: Partial<Tournament> = {}): Tournament {
  return {
    id: "t1",
    organization_id: "org1",
    name: "Summer Knockout",
    slug: "summer-knockout",
    sport: "football",
    format: "knockout",
    status: "registration_open",
    participant_type: "team",
    description: null,
    banner_url: null,
    prize_pool: null,
    currency: "INR",
    max_participants: 16,
    min_participants: null,
    registration_opens_at: null,
    registration_closes_at: null,
    starts_at: null,
    ends_at: null,
    venue: null,
    city: null,
    country: null,
    rules: null,
    created_by: null,
    created_at: new Date().toISOString(),
    updated_at: new Date().toISOString(),
    registration_counts: { ...EMPTY_REGISTRATION_COUNTS },
    ...overrides,
  }
}

function withCounts(overrides: Partial<RegistrationCounts>): RegistrationCounts {
  return { ...EMPTY_REGISTRATION_COUNTS, ...overrides }
}

function makeRegistration(
  overrides: Partial<TournamentRegistration> = {},
): TournamentRegistration {
  return {
    id: "r1",
    tournament_id: "t1",
    organization_id: "org1",
    team_id: "team1",
    player_id: null,
    seed_number: null,
    status: "pending",
    registered_by: null,
    registered_at: new Date().toISOString(),
    approved_by: null,
    approved_at: null,
    notes: null,
    created_at: new Date().toISOString(),
    updated_at: new Date().toISOString(),
    ...overrides,
  }
}

describe("analyzeEligibility — tournament state", () => {
  it("allows registration when open with free capacity and no duplicate", () => {
    expect(analyzeEligibility(makeTournament(), null)).toEqual({ canRegister: true })
  })

  it.each([
    ["draft", "hasn't been published"],
    ["registration_closed", "closed"],
    ["ongoing", "in progress"],
    ["completed", "ended"],
    ["cancelled", "cancelled"],
  ] as const)("blocks %s tournaments", (status, fragment) => {
    const result = analyzeEligibility(makeTournament({ status }), null)
    expect(result.canRegister).toBe(false)
    expect(result.reason).toContain(fragment)
  })

  it("blocks before the registration window opens", () => {
    const now = new Date("2026-06-01T10:00:00Z")
    const result = analyzeEligibility(
      makeTournament({ registration_opens_at: "2026-06-02T00:00:00Z" }),
      null,
      now,
    )
    expect(result.canRegister).toBe(false)
    expect(result.reason).toContain("Registrations open on")
  })

  it("blocks after the registration window closes", () => {
    const now = new Date("2026-06-10T10:00:00Z")
    const result = analyzeEligibility(
      makeTournament({ registration_closes_at: "2026-06-09T00:00:00Z" }),
      null,
      now,
    )
    expect(result.canRegister).toBe(false)
    expect(result.reason).toContain("window has closed")
  })
})

describe("analyzeEligibility — capacity boundary (pending + approved)", () => {
  it("blocks when ACTIVE registrations reach max, even if approved alone is below it", () => {
    // 6 approved + 10 pending = 16 active. Backend would 409 here; the
    // approved-only heuristic would wrongly claim eligibility.
    const result = analyzeEligibility(
      makeTournament({
        max_participants: 16,
        registration_counts: withCounts({ approved: 6, pending: 10, active: 16, total: 16 }),
      }),
      null,
    )
    expect(result.canRegister).toBe(false)
    expect(result.reason).toContain("16/16")
  })

  it("allows when active is one below max", () => {
    const result = analyzeEligibility(
      makeTournament({
        max_participants: 16,
        registration_counts: withCounts({ approved: 10, pending: 5, active: 15, total: 15 }),
      }),
      null,
    )
    expect(result.canRegister).toBe(true)
  })

  it("never blocks on capacity for uncapped tournaments", () => {
    const result = analyzeEligibility(
      makeTournament({
        max_participants: null,
        registration_counts: withCounts({ approved: 500, pending: 100, active: 600, total: 600 }),
      }),
      null,
    )
    expect(result.canRegister).toBe(true)
  })
})

describe("analyzeEligibility — duplicates block in ANY status (backend Rule 4)", () => {
  it.each(["pending", "approved", "rejected", "withdrawn", "disqualified"] as const)(
    "blocks when an existing registration is %s",
    (status) => {
      const result = analyzeEligibility(
        makeTournament(),
        makeRegistration({ status }),
      )
      expect(result.canRegister).toBe(false)
      expect(result.reason).toContain("already registered")
    },
  )
})

describe("getPickerOverflowHint — >page-size handling", () => {
  it("returns null when everything fits on one page", () => {
    expect(getPickerOverflowHint(15, 15)).toBeNull()
    expect(getPickerOverflowHint(20, 20)).toBeNull()
  })

  it("tells the user to narrow the search when the server holds more results", () => {
    expect(getPickerOverflowHint(250, 20)).toBe(
      "Showing first 20 of 250 — type to narrow the list.",
    )
  })
})
