import { describe, it, expect } from "vitest"
import { screen } from "@testing-library/react"
import { renderWithProviders } from "@/test/test-utils"
import {
  coerceFixtureValues,
  rfc3339ToLocal,
  FixtureForm,
  type FixtureFormValues,
  type CoercedFixtureValues,
} from "../fixture-form"
import {
  registrationsToParticipants,
  buildCreateMatchBody,
  buildUpdateMatchBody,
} from "../fixture-mapping"
import type { TournamentRegistration } from "@/types/api/tournament-registrations"

// ── coerceFixtureValues ────────────────────────────────────────────────────────

describe("coerceFixtureValues", () => {
  function values(overrides: Partial<FixtureFormValues> = {}): FixtureFormValues {
    return {
      home_participant_id: "h",
      away_participant_id: "a",
      scheduled_at: "2026-07-01T15:30",
      venue: "",
      round_name: "",
      round_number: "",
      match_number: "",
      ...overrides,
    }
  }

  it("converts datetime-local to an RFC3339 UTC instant", () => {
    const out = coerceFixtureValues(values({ scheduled_at: "2026-07-01T15:30" }))
    // The exact UTC offset is environment-dependent; assert it is a valid ISO instant.
    expect(out.scheduledAt).toBe(new Date("2026-07-01T15:30").toISOString())
    expect(Number.isNaN(Date.parse(out.scheduledAt))).toBe(false)
  })

  it("maps blank optional fields to undefined and parses numbers", () => {
    const out = coerceFixtureValues(values({ round_number: "2", match_number: "5", venue: "Court 1" }))
    expect(out).toMatchObject({
      homeId: "h",
      awayId: "a",
      venue: "Court 1",
      roundNumber: 2,
      matchNumber: 5,
    })
    expect(out.roundName).toBeUndefined()
  })
})

describe("rfc3339ToLocal", () => {
  it("returns empty string for nullish input", () => {
    expect(rfc3339ToLocal(null)).toBe("")
    expect(rfc3339ToLocal(undefined)).toBe("")
  })
  it("formats to datetime-local shape", () => {
    expect(rfc3339ToLocal("2026-07-01T10:00:00Z")).toMatch(/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}$/)
  })
})

// ── registrationsToParticipants ────────────────────────────────────────────────

function reg(overrides: Partial<TournamentRegistration>): TournamentRegistration {
  return {
    id: "r",
    tournament_id: "t",
    organization_id: "o",
    team_id: null,
    player_id: null,
    team_name: null,
    player_name: null,
    seed_number: null,
    status: "approved",
    registered_by: null,
    registered_at: "2026-06-01T00:00:00Z",
    approved_by: null,
    approved_at: null,
    notes: null,
    created_at: "2026-06-01T00:00:00Z",
    updated_at: "2026-06-01T00:00:00Z",
    ...overrides,
  }
}

describe("registrationsToParticipants", () => {
  it("maps team registrations to team identities", () => {
    const out = registrationsToParticipants(
      [reg({ team_id: "tm1", team_name: "Raiders" }), reg({ team_id: "tm2", team_name: "Kings" })],
      true,
    )
    expect(out).toEqual([
      { id: "tm1", name: "Raiders" },
      { id: "tm2", name: "Kings" },
    ])
  })

  it("maps individual registrations to player identities", () => {
    const out = registrationsToParticipants(
      [reg({ player_id: "p1", player_name: "Arjun" })],
      false,
    )
    expect(out).toEqual([{ id: "p1", name: "Arjun" }])
  })

  it("drops registrations missing the relevant id", () => {
    const out = registrationsToParticipants([reg({ player_id: "p1", player_name: "Arjun" })], true)
    expect(out).toEqual([])
  })
})

// ── buildCreateMatchBody / buildUpdateMatchBody ────────────────────────────────

const coerced: CoercedFixtureValues = {
  homeId: "h",
  awayId: "a",
  scheduledAt: "2026-07-01T10:00:00.000Z",
  venue: "Court 1",
  roundName: undefined,
  roundNumber: 2,
  matchNumber: undefined,
}

describe("buildCreateMatchBody", () => {
  it("routes ids to team slots for team tournaments and never leaks player slots", () => {
    const body = buildCreateMatchBody("tour1", true, coerced)
    expect(body.tournament_id).toBe("tour1")
    expect(body.home_team_id).toBe("h")
    expect(body.away_team_id).toBe("a")
    expect(body.home_player_id).toBeUndefined()
    expect(body.away_player_id).toBeUndefined()
    expect(body.scheduled_at).toBe("2026-07-01T10:00:00.000Z")
  })

  it("routes ids to player slots for individual tournaments", () => {
    const body = buildCreateMatchBody("tour1", false, coerced)
    expect(body.home_player_id).toBe("h")
    expect(body.away_player_id).toBe("a")
    expect(body.home_team_id).toBeUndefined()
    expect(body.away_team_id).toBeUndefined()
  })
})

describe("buildUpdateMatchBody", () => {
  it("sends cleared optional fields as null", () => {
    const body = buildUpdateMatchBody(true, { ...coerced, venue: undefined, roundNumber: undefined })
    expect(body.venue).toBeNull()
    expect(body.round_number).toBeNull()
    expect(body.home_team_id).toBe("h")
  })
})

// ── FixtureForm guards ─────────────────────────────────────────────────────────

describe("FixtureForm", () => {
  it("disables submit and warns when fewer than two participants exist", () => {
    renderWithProviders(
      <FixtureForm
        participants={[{ id: "only", name: "Solo" }]}
        participantNoun="team"
        isPending={false}
        onSubmit={() => {}}
      />,
    )
    expect(
      screen.getByText(/At least two approved teams are required/i),
    ).toBeInTheDocument()
    expect(screen.getByRole("button", { name: /create fixture/i })).toBeDisabled()
  })

  it("renders home and away selects with the participant noun", () => {
    renderWithProviders(
      <FixtureForm
        participants={[
          { id: "a", name: "Alpha" },
          { id: "b", name: "Bravo" },
        ]}
        participantNoun="player"
        isPending={false}
        onSubmit={() => {}}
      />,
    )
    expect(screen.getByText("Home player")).toBeInTheDocument()
    expect(screen.getByText("Away player")).toBeInTheDocument()
  })
})
