import type { Role, Permission } from "@/types/common"
import { ROLE_PERMISSIONS } from "@/types/common"
import type { JwtClaims } from "@/types/api/auth"

export function hasPermission(role: Role | null, perm: Permission): boolean {
  if (!role) return false
  const perms = ROLE_PERMISSIONS[role]
  if (perms[0] === "*") return true
  return (perms as Permission[]).includes(perm)
}

/**
 * Whether the authenticated user may enter the organizer application shell
 * (the `(app)/[orgSlug]` route tree). This mirrors the backend RequireOrgScope
 * boundary: only organizer-context and platform users belong in the org shell.
 *
 * - organizer users carry an `organizationId` in their token → allowed.
 * - platform users carry an EMPTY `organizationId` but administer any org →
 *   allowed via `scope === "platform"` (and the legacy `platform_admin` role,
 *   for tokens minted before the scope claim existed).
 * - onboarding/player users have no org context → denied (sent to /welcome).
 */
export function hasOrgContext(claims: JwtClaims | null): boolean {
  if (!claims) return false
  if (claims.organizationId) return true
  return claims.scope === "platform" || claims.role === "platform_admin"
}

export const ROLE_LABELS: Record<Role, string> = {
  onboarding: "Onboarding",
  platform_admin: "Platform Admin",
  org_owner: "Owner",
  org_admin: "Admin",
  team_manager: "Team Manager",
  coach: "Coach",
  scorer: "Scorer",
  viewer: "Viewer",
}

export const ROLE_VARIANTS: Record<Role, "default" | "secondary" | "outline"> = {
  onboarding: "outline",
  platform_admin: "default",
  org_owner: "default",
  org_admin: "secondary",
  team_manager: "secondary",
  coach: "outline",
  scorer: "outline",
  viewer: "outline",
}
