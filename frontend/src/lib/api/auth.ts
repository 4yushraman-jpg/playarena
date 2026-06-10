"use client"

import api from "./client"
import type {
  LoginRequest,
  RegisterRequest,
  LogoutRequest,
  ForgotPasswordRequest,
  ResetPasswordRequest,
  ResendVerificationRequest,
  TokenResponse,
  RegisterResponse,
  AuthUser,
  OrgRequiredResponse,
} from "@/types/api/auth"

const BASE = "/api/v1/auth"

export const authApi = {
  login: (data: LoginRequest) =>
    api.post<TokenResponse>(`${BASE}/login`, data),

  loginWithOrg: (data: LoginRequest & { organization_id: string }) =>
    api.post<TokenResponse>(`${BASE}/login`, data),

  // Returns 409 OrgRequiredResponse if user belongs to multiple orgs.
  // The caller must handle AxiosError with status 409 separately.
  loginAny: (data: LoginRequest) =>
    api.post<TokenResponse | OrgRequiredResponse>(`${BASE}/login`, data),

  register: (data: RegisterRequest) =>
    api.post<RegisterResponse>(`${BASE}/register`, data),

  verifyEmail: (token: string) =>
    api.get<{ message: string }>(`${BASE}/verify-email`, { params: { token } }),

  forgotPassword: (data: ForgotPasswordRequest) =>
    api.post<{ message: string; reset_token?: string }>(`${BASE}/forgot-password`, data),

  resetPassword: (data: ResetPasswordRequest) =>
    api.post<{ message: string }>(`${BASE}/reset-password`, data),

  resendVerification: (data: ResendVerificationRequest) =>
    api.post<{ message: string }>(`${BASE}/resend-verification`, data),

  logout: (data: LogoutRequest) =>
    api.post<{ message: string }>(`${BASE}/logout`, data),

  me: () =>
    api.get<AuthUser>(`${BASE}/me`),
}
