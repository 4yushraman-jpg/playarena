export type UserStatus = "active" | "pending_verification" | "suspended" | "inactive"

export interface User {
  id: string
  email: string
  username: string
  full_name: string
  status: UserStatus
  email_verified_at: string | null
  last_login_at: string | null
  last_login_ip: string | null
  created_at: string
  updated_at: string
}

export interface UpdateUserRequest {
  full_name?: string
  username?: string
}

export interface ChangePasswordRequest {
  current_password: string
  new_password: string
}

export interface UserListParams {
  limit?: number
  offset?: number
  search?: string
}
