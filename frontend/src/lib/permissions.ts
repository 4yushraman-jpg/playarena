import type { Role, Permission } from "@/types/common"
import { ROLE_PERMISSIONS } from "@/types/common"

export function hasPermission(role: Role | null, perm: Permission): boolean {
  if (!role) return false
  const perms = ROLE_PERMISSIONS[role]
  if (perms[0] === "*") return true
  return (perms as Permission[]).includes(perm)
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
