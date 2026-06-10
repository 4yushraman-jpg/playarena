// Backend members.RoleGrant
export interface RoleGrant {
  grant_id: string
  role_slug: string
  role_name: string
  granted_at: string
  expires_at: string | null
  granted_by: string | null
}

// Backend members.MemberResponse — user + all active roles in the org
export interface OrgMember {
  user_id: string
  email: string
  username: string
  first_name: string
  last_name: string
  user_status: string
  roles: RoleGrant[]
}

// POST /organizations/{slug}/members/{userID}/roles
// user_id is in the URL path, not the body
export interface GrantRoleRequest {
  role_slug: string
  expires_at?: string  // RFC3339; omit for no expiry
}

export interface MemberListParams {
  limit?: number
  offset?: number
}
