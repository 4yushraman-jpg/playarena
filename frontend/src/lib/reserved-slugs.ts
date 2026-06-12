// Reserved top-level path segments that must never be treated as an org slug.
// GP-1 introduced non-org routes (/me, /players); an org whose slug collides
// with one of these would shadow them. The backend prefixes generated org
// slugs to avoid this; the frontend guards the [orgSlug] route as defense in
// depth.
export const RESERVED_SLUGS = new Set<string>([
  "me",
  "player",
  "players",
  "onboarding",
  "auth",
  "api",
  "login",
  "register",
  "admin",
])

export function isReservedSlug(slug: string): boolean {
  return RESERVED_SLUGS.has(slug.toLowerCase())
}
