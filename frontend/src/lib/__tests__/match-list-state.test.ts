import { describe, it, expect } from "vitest"
import {
  parseMatchListState,
  serializeMatchListState,
  DEFAULT_MATCH_LIST_STATE,
} from "@/lib/match-list-state"

describe("parseMatchListState", () => {
  it("returns defaults for empty params", () => {
    expect(parseMatchListState(new URLSearchParams())).toEqual(DEFAULT_MATCH_LIST_STATE)
  })

  it("parses valid status and 1-based page into 0-based", () => {
    const state = parseMatchListState(new URLSearchParams("q=final&status=live&page=3"))
    expect(state).toEqual({ search: "final", status: "live", page: 2 })
  })

  it("ignores an unknown status", () => {
    const state = parseMatchListState(new URLSearchParams("status=bogus"))
    expect(state.status).toBe("all")
  })

  it("clamps page below 1 to 0", () => {
    expect(parseMatchListState(new URLSearchParams("page=0")).page).toBe(0)
    expect(parseMatchListState(new URLSearchParams("page=-5")).page).toBe(0)
  })
})

describe("serializeMatchListState", () => {
  it("omits defaults for a clean canonical URL", () => {
    expect(serializeMatchListState(DEFAULT_MATCH_LIST_STATE)).toBe("")
  })

  it("serializes non-default values with 1-based page", () => {
    const qs = serializeMatchListState({ search: "ko", status: "completed", page: 2 })
    const params = new URLSearchParams(qs)
    expect(params.get("q")).toBe("ko")
    expect(params.get("status")).toBe("completed")
    expect(params.get("page")).toBe("3")
  })

  it("round-trips through parse", () => {
    const original = { search: "semi", status: "scheduled" as const, page: 4 }
    const reparsed = parseMatchListState(
      new URLSearchParams(serializeMatchListState(original)),
    )
    expect(reparsed).toEqual(original)
  })
})
