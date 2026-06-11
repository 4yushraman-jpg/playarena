import { describe, it, expect } from "vitest"
import {
  DEFAULT_LIST_STATE,
  parseListState,
  serializeListState,
} from "../tournament-list-state"

describe("tournament directory URL state", () => {
  it("parses defaults from an empty query string", () => {
    expect(parseListState(new URLSearchParams())).toEqual(DEFAULT_LIST_STATE)
  })

  it("round-trips a fully populated state through the URL", () => {
    const state = { search: "summer cup", status: "registration_open" as const, page: 2 }
    const qs = serializeListState(state)
    expect(parseListState(new URLSearchParams(qs))).toEqual(state)
  })

  it("omits default values so the canonical URL stays clean", () => {
    expect(serializeListState(DEFAULT_LIST_STATE)).toBe("")
  })

  it("uses 1-based page numbers in the URL", () => {
    expect(serializeListState({ search: "", status: "all", page: 1 })).toBe("page=2")
    expect(parseListState(new URLSearchParams("page=2")).page).toBe(1)
  })

  it("ignores invalid status values", () => {
    expect(parseListState(new URLSearchParams("status=bogus")).status).toBe("all")
  })

  it("ignores invalid and out-of-range page values", () => {
    expect(parseListState(new URLSearchParams("page=abc")).page).toBe(0)
    expect(parseListState(new URLSearchParams("page=-3")).page).toBe(0)
    expect(parseListState(new URLSearchParams("page=0")).page).toBe(0)
  })

  it("preserves search text including spaces", () => {
    const qs = serializeListState({ search: "knock out 2025", status: "all", page: 0 })
    expect(parseListState(new URLSearchParams(qs)).search).toBe("knock out 2025")
  })
})
