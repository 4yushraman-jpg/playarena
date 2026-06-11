import { describe, it, expect, vi, beforeEach } from "vitest"
import { waitFor } from "@testing-library/react"
import { QueryClient } from "@tanstack/react-query"
import { renderHookWithProviders, makeTestQueryClient } from "@/test/test-utils"
import { useUpdateRegistration, useRegisterParticipant } from "../use-registrations"
import { registrationsApi } from "@/lib/api/registrations"
import { tournamentKeys } from "@/lib/query-keys"
import { toast } from "sonner"
import type { TournamentRegistration } from "@/types/api/tournament-registrations"

vi.mock("@/lib/api/registrations", () => ({
  registrationsApi: {
    list: vi.fn(),
    getById: vi.fn(),
    register: vi.fn(),
    update: vi.fn(),
    withdraw: vi.fn(),
  },
}))

vi.mock("sonner", () => ({
  toast: { success: vi.fn(), error: vi.fn() },
}))

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
    status: "approved",
    registered_by: null,
    registered_at: new Date().toISOString(),
    approved_by: "u1",
    approved_at: new Date().toISOString(),
    notes: null,
    created_at: new Date().toISOString(),
    updated_at: new Date().toISOString(),
    ...overrides,
  }
}

beforeEach(() => {
  vi.clearAllMocks()
})

describe("useUpdateRegistration — approval", () => {
  it("invalidates the tournament detail (capacity counts) and registration lists", async () => {
    vi.mocked(registrationsApi.update).mockResolvedValue({
      data: makeRegistration({ status: "approved" }),
    } as never)

    const client = makeTestQueryClient()
    const invalidateSpy = vi.spyOn(client, "invalidateQueries")

    const { result } = renderHookWithProviders(
      () => useUpdateRegistration("my-org", "t1"),
      { client },
    )

    result.current.mutate({ registrationId: "r1", body: { status: "approved" } })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    // Registration lists under the tournament refresh (tab contents)
    expect(invalidateSpy).toHaveBeenCalledWith(
      expect.objectContaining({ queryKey: tournamentKeys.registrations("my-org", "t1") }),
    )
    // Tournament detail refreshes — it carries registration_counts, which
    // drive the capacity bar. This was review finding P1-08.
    expect(invalidateSpy).toHaveBeenCalledWith(
      expect.objectContaining({ queryKey: tournamentKeys.detail("my-org", "t1") }),
    )
    // Directory lists refresh so the capacity column stays correct.
    expect(invalidateSpy).toHaveBeenCalledWith(
      expect.objectContaining({ queryKey: tournamentKeys.lists("my-org") }),
    )
  })

  it("announces success with status-specific copy (P1-07)", async () => {
    vi.mocked(registrationsApi.update).mockResolvedValue({
      data: makeRegistration({ status: "approved" }),
    } as never)

    const { result } = renderHookWithProviders(() => useUpdateRegistration("my-org", "t1"))
    result.current.mutate({ registrationId: "r1", body: { status: "approved" } })

    await waitFor(() => expect(toast.success).toHaveBeenCalledWith("Registration approved"))
  })

  it("announces rejection with its own copy", async () => {
    vi.mocked(registrationsApi.update).mockResolvedValue({
      data: makeRegistration({ status: "rejected" }),
    } as never)

    const { result } = renderHookWithProviders(() => useUpdateRegistration("my-org", "t1"))
    result.current.mutate({ registrationId: "r1", body: { status: "rejected" } })

    await waitFor(() => expect(toast.success).toHaveBeenCalledWith("Registration rejected"))
  })

  it("surfaces an error toast when the mutation fails", async () => {
    vi.mocked(registrationsApi.update).mockRejectedValue(new Error("boom"))

    const { result } = renderHookWithProviders(() => useUpdateRegistration("my-org", "t1"))
    result.current.mutate({ registrationId: "r1", body: { status: "approved" } })

    await waitFor(() => expect(toast.error).toHaveBeenCalled())
    expect(toast.success).not.toHaveBeenCalled()
  })

  it("partial-matching actually hits a live registration list query key", async () => {
    // Regression guard for the params-vs-undefined key mismatch: an
    // invalidation filter built from the registrations ROOT must match a
    // stored registrationList key that ends in a params object.
    vi.mocked(registrationsApi.update).mockResolvedValue({
      data: makeRegistration({ status: "approved" }),
    } as never)

    // gcTime must be > 0 here: a seeded query with no observers would be
    // garbage-collected before the invalidation assertion runs.
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false, gcTime: Infinity } },
    })
    const listKey = tournamentKeys.registrationList("my-org", "t1", {
      limit: 50,
      offset: 0,
      status: "pending",
    })
    client.setQueryData(listKey, { registrations: [], total: 0, limit: 50, offset: 0 })

    const { result } = renderHookWithProviders(
      () => useUpdateRegistration("my-org", "t1"),
      { client },
    )
    result.current.mutate({ registrationId: "r1", body: { status: "approved" } })
    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(client.getQueryState(listKey)?.isInvalidated).toBe(true)
  })
})

describe("useRegisterParticipant", () => {
  it("invalidates registrations, detail, and directory lists on success", async () => {
    vi.mocked(registrationsApi.register).mockResolvedValue({
      data: makeRegistration({ status: "pending" }),
    } as never)

    const client = makeTestQueryClient()
    const invalidateSpy = vi.spyOn(client, "invalidateQueries")

    const { result } = renderHookWithProviders(
      () => useRegisterParticipant("my-org", "t1"),
      { client },
    )
    result.current.mutate({ team_id: "team1" })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(invalidateSpy).toHaveBeenCalledWith(
      expect.objectContaining({ queryKey: tournamentKeys.registrations("my-org", "t1") }),
    )
    expect(invalidateSpy).toHaveBeenCalledWith(
      expect.objectContaining({ queryKey: tournamentKeys.detail("my-org", "t1") }),
    )
    expect(toast.success).toHaveBeenCalledWith("Registration submitted")
  })
})
