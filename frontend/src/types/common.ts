// Shared API primitives

export interface PaginatedResponse<T> {
  data: T[]
  total: number
  limit: number
  offset: number
}

export interface ApiErrorResponse {
  error: string
  fields?: Record<string, string>
  code?: string
}

// ── RBAC ─────────────────────────────────────────────────────────────────────

export type Role =
  | "onboarding"
  | "platform_admin"
  | "org_owner"
  | "org_admin"
  | "team_manager"
  | "coach"
  | "scorer"
  | "viewer"

export type Permission =
  | "organization.create"
  | "organization.update"
  | "organization.delete"
  | "user.manage"
  | "role.assign"
  | "team.create"
  | "team.update"
  | "team.delete"
  | "player.create"
  | "player.update"
  | "player.delete"
  | "tournament.create"
  | "tournament.update"
  | "tournament.delete"
  | "match.create"
  | "match.update"
  | "match.delete"
  | "match.score"
  | "media.upload"
  | "media.update"
  | "media.delete"
  | "notification.manage"
  | "webhook.create"
  | "webhook.read"
  | "webhook.update"
  | "webhook.delete"

// Permissions granted to each role — mirrors the backend permission matrix exactly.
// Used for client-side UI gating only; the backend still enforces authorization.
export const ROLE_PERMISSIONS: Record<Role, Permission[] | ["*"]> = {
  onboarding: ["organization.create"],
  platform_admin: ["*"],
  org_owner: [
    "organization.update", "organization.delete",
    "user.manage", "role.assign",
    "team.create", "team.update", "team.delete",
    "player.create", "player.update", "player.delete",
    "tournament.create", "tournament.update", "tournament.delete",
    "match.create", "match.update", "match.delete", "match.score",
    "media.upload", "media.update", "media.delete",
    "notification.manage",
    "webhook.create", "webhook.read", "webhook.update", "webhook.delete",
  ],
  org_admin: [
    "organization.update",
    "user.manage", "role.assign",
    "team.create", "team.update", "team.delete",
    "player.create", "player.update", "player.delete",
    "tournament.create", "tournament.update", "tournament.delete",
    "match.create", "match.update", "match.delete", "match.score",
    "media.upload", "media.update", "media.delete",
    "notification.manage",
    "webhook.create", "webhook.read", "webhook.update", "webhook.delete",
  ],
  team_manager: [
    "team.create", "team.update", "team.delete",
    "player.create", "player.update", "player.delete",
    "media.upload", "media.update", "media.delete",
  ],
  coach: [
    "player.update",
    "match.update",
    "media.upload", "media.update", "media.delete",
  ],
  scorer: [
    "match.update", "match.score",
  ],
  viewer: [],
}

export type SortDirection = "asc" | "desc"

export interface ListParams {
  limit?: number
  offset?: number
  search?: string
}
