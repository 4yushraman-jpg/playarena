import { describe, it, expect, vi, beforeEach } from "vitest"
import { renderHookWithProviders, makeTestQueryClient } from "@/test/test-utils"
import { useParticipantNames } from "@/hooks/use-participant-names"
import { teamKeys, playerKeys } from "@/lib/query-keys"

vi.mock("@/lib/api/teams", () => ({
  teamsApi: { list: vi.fn(), getById: vi.fn(), create: vi.fn(), update: vi.fn(), delete: vi.fn(), listMembers: vi.fn(), addMember: vi.fn(), removeMember: vi.fn() },
}))
vi.mock("@/lib/api/players", () => ({
  playersApi: { list: vi.fn(), getById: vi.fn(), create: vi.fn(), update: vi.fn(), delete: vi.fn() },
}))

import { teamsApi } from "@/lib/api/teams"
import { playersApi } from "@/lib/api/players"

beforeEach(() => {
  vi.clearAllMocks()
})

describe("useParticipantNames", () => {
  it("shows a neutral placeholder (not a raw UUID) while the maps are loading", () => {
    // Teams query never resolves → hook stays in loading state.
    vi.mocked(teamsApi.list).mockReturnValue(new Promise(() => {}) as never)
    vi.mocked(playersApi.list).mockResolvedValue({ data: { players: [], total: 0, limit: 200, offset: 0 } } as never)

    const { result } = renderHookWithProviders(() => useParticipantNames("test-org"))
    expect(result.current.isLoading).toBe(true)
    expect(result.current.resolve("d4f9a1b2-c3d4-e5f6-a7b8-c9d0e1f2a3b4", null)).toBe("…")
  })

  it("resolves a known id to its name and degrades an unknown id to a short id once loaded", () => {
    vi.mocked(teamsApi.list).mockResolvedValue({ data: { teams: [], total: 0, limit: 200, offset: 0 } } as never)
    vi.mocked(playersApi.list).mockResolvedValue({ data: { players: [], total: 0, limit: 200, offset: 0 } } as never)

    const client = makeTestQueryClient()
    client.setQueryData(teamKeys.list("test-org", { limit: 200 }), {
      teams: [{ id: "tm-raiders", name: "Raiders" }],
      total: 1, limit: 200, offset: 0,
    })
    client.setQueryData(playerKeys.list("test-org", { limit: 200 }), {
      players: [], total: 0, limit: 200, offset: 0,
    })

    const { result } = renderHookWithProviders(() => useParticipantNames("test-org"), { client })
    expect(result.current.isLoading).toBe(false)
    expect(result.current.resolve("tm-raiders", null)).toBe("Raiders")
    // Loaded but absent from the bounded map → short id, never a placeholder.
    expect(result.current.resolve("d4f9a1b2-c3d4-e5f6-a7b8-c9d0e1f2a3b4", null)).toBe("d4f9a1b2…")
    // No participant set.
    expect(result.current.resolve(null, null)).toBe("TBD")
  })
})
