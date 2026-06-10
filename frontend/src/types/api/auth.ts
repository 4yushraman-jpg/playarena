import type { Role } from "../common"

// ── Requests ──────────────────────────────────────────────────────────────────

export interface LoginRequest {
  email: string
  password: string
  organization_id?: string
}

export interface RegisterRequest {
  email: string
  username: string
  password: string
  full_name: string
}

export interface RefreshRequest {
  refresh_token: string
  organization_id?: string
}

export interface LogoutRequest {
  refresh_token: string
}

export interface ForgotPasswordRequest {
  email: string
}

export interface ResetPasswordRequest {
  token: string
  password: string
}

export interface ResendVerificationRequest {
  email: string
}

// ── Responses ─────────────────────────────────────────────────────────────────

export interface TokenResponse {
  access_token: string
  refresh_token: string
  expires_in: number
  token_type: "Bearer"
}

export interface RegisterResponse {
  id: string
  email: string
  username: string
  message: string
  verification_token?: string // dev only
}

// Minimal org representation returned by the 409 org-picker response.
// Backend auth.OrgSummary: { id, name, slug } — no role field.
export interface OrgSummary {
  id: string
  name: string
  slug: string
}

// HTTP 409 body when multi-org user logs in without specifying an org.
export interface OrgRequiredResponse {
  error: string
  code: string
  organizations: OrgSummary[]
}

// Response from GET /api/v1/auth/me.
// Backend meResponse: { id, email, username, full_name, status, role, organization_id }
export interface AuthUser {
  id: string
  email: string
  username: string
  full_name: string
  status: "active" | "pending_verification" | "suspended" | "inactive"
  role: Role
  organization_id: string
}

// Decoded JWT claims stored in Zustand.
// Backend JWTClaims: { user_id, organization_id, role, email, exp }
// organizationId is null for platform admins whose JWT carries no org context.
export interface JwtClaims {
  userId: string
  email: string
  organizationId: string | null
  role: Role
  exp: number
}
