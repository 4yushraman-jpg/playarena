"use client"

import axios, { type AxiosError, type InternalAxiosRequestConfig } from "axios"
import type { TokenResponse } from "@/types/api/auth"

const BASE_URL = process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:8080"

export const api = axios.create({
  baseURL: BASE_URL,
  headers: { "Content-Type": "application/json" },
  withCredentials: false,
})

// ── Token management ──────────────────────────────────────────────────────────
// Access token lives in sessionStorage (tab-scoped). Refresh token lives in
// localStorage (persists across tabs). Both are accessed through these helpers
// so the stores stay decoupled from import cycles (auth.store imports client,
// client must not import auth.store).

const ACCESS_TOKEN_KEY = "pa_access_token"
const REFRESH_TOKEN_KEY = "pa_refresh_token"

export const tokenManager = {
  getAccessToken(): string | null {
    if (typeof window === "undefined") return null
    return sessionStorage.getItem(ACCESS_TOKEN_KEY)
  },
  setAccessToken(token: string): void {
    sessionStorage.setItem(ACCESS_TOKEN_KEY, token)
  },
  getRefreshToken(): string | null {
    if (typeof window === "undefined") return null
    return localStorage.getItem(REFRESH_TOKEN_KEY)
  },
  setRefreshToken(token: string): void {
    localStorage.setItem(REFRESH_TOKEN_KEY, token)
  },
  clearAll(): void {
    sessionStorage.removeItem(ACCESS_TOKEN_KEY)
    localStorage.removeItem(REFRESH_TOKEN_KEY)
  },
}

// ── Request interceptor — attach access token ─────────────────────────────────

api.interceptors.request.use((config: InternalAxiosRequestConfig) => {
  const token = tokenManager.getAccessToken()
  if (token && config.headers) {
    config.headers.Authorization = `Bearer ${token}`
  }
  return config
})

// ── Response interceptor — handle auth errors ─────────────────────────────────

let refreshPromise: Promise<string | null> | null = null

export async function attemptTokenRefresh(): Promise<string | null> {
  if (refreshPromise) return refreshPromise

  refreshPromise = (async () => {
    const refreshToken = tokenManager.getRefreshToken()
    if (!refreshToken) return null

    try {
      const { data } = await axios.post<TokenResponse>(
        `${BASE_URL}/api/v1/auth/refresh`,
        { refresh_token: refreshToken },
        { headers: { "Content-Type": "application/json" } },
      )
      tokenManager.setAccessToken(data.access_token)
      tokenManager.setRefreshToken(data.refresh_token)
      return data.access_token
    } catch {
      tokenManager.clearAll()
      return null
    } finally {
      refreshPromise = null
    }
  })()

  return refreshPromise
}

interface RetryableConfig extends InternalAxiosRequestConfig {
  _retry?: boolean
}

api.interceptors.response.use(
  (response) => response,
  async (error: AxiosError) => {
    const config = error.config as RetryableConfig | undefined
    const status = error.response?.status

    // 401 — try silent refresh once, then redirect to login
    if (status === 401 && config && !config._retry) {
      config._retry = true
      const newToken = await attemptTokenRefresh()
      if (newToken) {
        config.headers = config.headers ?? {}
        config.headers.Authorization = `Bearer ${newToken}`
        return api(config)
      }
      // Refresh failed — clear state and redirect
      tokenManager.clearAll()
      if (typeof window !== "undefined") {
        window.location.href = "/login"
      }
      return Promise.reject(error)
    }

    // 409 — multi-org selection required; handled per-callsite via error
    // Other errors propagate as-is
    return Promise.reject(error)
  },
)

export default api
