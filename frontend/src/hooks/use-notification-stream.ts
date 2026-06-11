"use client"

import { useEffect, useRef, useCallback } from "react"
import { useRouter } from "next/navigation"
import { useQueryClient } from "@tanstack/react-query"
import { tokenManager, attemptTokenRefresh } from "@/lib/api/client"
import { useAuthStore } from "@/stores/auth.store"
import { getQueryClient } from "@/lib/api/query-client"
import { matchKeys, tournamentKeys, notificationKeys } from "@/lib/query-keys"
import type { QueryClient } from "@tanstack/react-query"
import type { NotificationStreamEvent } from "@/types/api/notifications"

const BASE_URL = process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:8080"

// Backend SSE endpoint:
// GET /api/v1/organizations/{slug}/notifications/stream?token=<jwt>
// Token must be in query param — EventSource cannot send Authorization headers.
// When the JWT expires the server sends event: auth_error. We then refresh the
// token explicitly before reconnecting to avoid an auth_error → reconnect loop.

const RECONNECT_DELAY_MS = 3_000
const MAX_RECONNECT_DELAY_MS = 30_000
const BACKOFF_FACTOR = 2

interface UseNotificationStreamOptions {
  orgSlug: string
  enabled?: boolean
}

export function useNotificationStream({ orgSlug, enabled = true }: UseNotificationStreamOptions) {
  const queryClient = useQueryClient()
  const router = useRouter()
  const esRef = useRef<EventSource | null>(null)
  const reconnectTimeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const reconnectDelayRef = useRef(RECONNECT_DELAY_MS)
  const mountedRef = useRef(true)
  // Stable ref to the latest connect function — used inside closures that would
  // otherwise capture a stale version, and avoids the forward-reference TDZ issue.
  const connectRef = useRef<() => void>(() => {})

  const disconnect = useCallback(() => {
    if (reconnectTimeoutRef.current) {
      clearTimeout(reconnectTimeoutRef.current)
      reconnectTimeoutRef.current = null
    }
    if (esRef.current) {
      esRef.current.close()
      esRef.current = null
    }
  }, [])

  const connect = useCallback(() => {
    if (!mountedRef.current || !enabled) return

    const token = tokenManager.getAccessToken()
    if (!token) return

    disconnect()

    const url = `${BASE_URL}/api/v1/organizations/${orgSlug}/notifications/stream?token=${encodeURIComponent(token)}`
    const es = new EventSource(url)
    esRef.current = es

    es.onopen = () => {
      reconnectDelayRef.current = RECONNECT_DELAY_MS
    }

    es.onmessage = (event) => {
      try {
        const data = JSON.parse(event.data) as NotificationStreamEvent
        queryClient.invalidateQueries({ queryKey: notificationKeys.all(orgSlug) })
        handleStreamEvent(data, orgSlug, queryClient)
      } catch {
        // Ignore malformed frames
      }
    }

    es.onerror = () => {
      es.close()
      esRef.current = null
      if (!mountedRef.current) return

      const delay = Math.min(reconnectDelayRef.current, MAX_RECONNECT_DELAY_MS)
      reconnectDelayRef.current = Math.min(delay * BACKOFF_FACTOR, MAX_RECONNECT_DELAY_MS)

      reconnectTimeoutRef.current = setTimeout(() => {
        if (mountedRef.current) connectRef.current()
      }, delay)
    }

    // auth_error is sent when the JWT in the query param has expired.
    // We must refresh the access token first; reconnecting with the same expired
    // token would create an infinite auth_error → reconnect loop.
    es.addEventListener("auth_error", () => {
      es.close()
      esRef.current = null
      if (!mountedRef.current) return

      attemptTokenRefresh().then((newToken) => {
        if (!mountedRef.current) return
        if (!newToken) {
          // Refresh failed — flush cache, clear store state, redirect to login.
          getQueryClient().clear()
          useAuthStore.getState().clearSession()
          router.replace("/login")
          return
        }
        reconnectTimeoutRef.current = setTimeout(() => {
          if (mountedRef.current) connectRef.current()
        }, 1_000)
      })
    })
  }, [orgSlug, enabled, disconnect, queryClient, router])

  useEffect(() => {
    mountedRef.current = true
    // Keep the ref in sync after each render so internal closures always call
    // the latest version of connect without capturing a stale closure.
    connectRef.current = connect
    if (enabled) connect()

    return () => {
      mountedRef.current = false
      disconnect()
    }
  }, [orgSlug, enabled, connect, disconnect])
}

// ── Cache invalidation per event type ────────────────────────────────────────

function handleStreamEvent(
  event: NotificationStreamEvent,
  orgSlug: string,
  queryClient: QueryClient,
) {
  switch (event.event_type) {
    case "match_started":
    case "match_completed":
    case "match_cancelled":
    case "match_abandoned":
      queryClient.invalidateQueries({ queryKey: matchKeys.all(orgSlug) })
      if (event.entity_id) {
        queryClient.invalidateQueries({ queryKey: matchKeys.detail(orgSlug, event.entity_id) })
        queryClient.invalidateQueries({ queryKey: matchKeys.score(orgSlug, event.entity_id) })
      }
      break
    case "tournament_status_changed":
      queryClient.invalidateQueries({ queryKey: tournamentKeys.all(orgSlug) })
      if (event.entity_id) {
        queryClient.invalidateQueries({ queryKey: tournamentKeys.detail(orgSlug, event.entity_id) })
        queryClient.invalidateQueries({ queryKey: tournamentKeys.standings(orgSlug, event.entity_id) })
      }
      break
    case "registration_approved":
    case "registration_rejected":
    case "registration_withdrawn": {
      // entity_id is the registration ID; tournament_id is in the payload
      const tournamentId = event.payload?.tournament_id as string | undefined
      if (tournamentId) {
        queryClient.invalidateQueries({
          queryKey: tournamentKeys.registrations(orgSlug, tournamentId),
        })
        queryClient.invalidateQueries({
          queryKey: tournamentKeys.detail(orgSlug, tournamentId),
        })
      }
      break
    }
    default:
      break
  }
}
