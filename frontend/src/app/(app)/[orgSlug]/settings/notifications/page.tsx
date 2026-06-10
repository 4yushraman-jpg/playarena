"use client"

import { useParams } from "next/navigation"
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query"
import { Loader2Icon } from "lucide-react"
import { toast } from "sonner"
import { Button } from "@/components/ui/button"
import { Switch } from "@/components/ui/switch"
import { Skeleton } from "@/components/ui/skeleton"
import { Separator } from "@/components/ui/separator"
import { notificationKeys } from "@/lib/query-keys"
import { notificationsApi } from "@/lib/api/notifications"
import { extractApiError } from "@/lib/api-error"
import type {
  NotificationEventType,
  NotificationChannel,
  NotificationPreference,
} from "@/types/api/notifications"

// ── Event type metadata ───────────────────────────────────────────────────────

interface EventTypeInfo {
  label: string
  description: string
  group: string
}

const EVENT_TYPE_INFO: Record<NotificationEventType, EventTypeInfo> = {
  match_created: {
    label: "Match Created",
    description: "A new match has been scheduled",
    group: "Matches",
  },
  match_started: {
    label: "Match Started",
    description: "A match goes live",
    group: "Matches",
  },
  match_completed: {
    label: "Match Completed",
    description: "A match finishes and results are final",
    group: "Matches",
  },
  match_cancelled: {
    label: "Match Cancelled",
    description: "A scheduled match is cancelled",
    group: "Matches",
  },
  match_abandoned: {
    label: "Match Abandoned",
    description: "A match is abandoned mid-game",
    group: "Matches",
  },
  tournament_status_changed: {
    label: "Tournament Status Changed",
    description: "A tournament moves to a new phase",
    group: "Tournaments",
  },
  registration_approved: {
    label: "Registration Approved",
    description: "Your registration is accepted",
    group: "Registrations",
  },
  registration_rejected: {
    label: "Registration Rejected",
    description: "Your registration is not accepted",
    group: "Registrations",
  },
  registration_withdrawn: {
    label: "Registration Withdrawn",
    description: "A registration is withdrawn",
    group: "Registrations",
  },
}

const ALL_EVENT_TYPES = Object.keys(EVENT_TYPE_INFO) as NotificationEventType[]
const GROUPS = ["Matches", "Tournaments", "Registrations"]

// ── Preference helpers ────────────────────────────────────────────────────────

function getEffectiveEnabled(
  preferences: NotificationPreference[],
  eventType: NotificationEventType,
  channel: NotificationChannel,
): boolean {
  const pref = preferences.find(
    (p) => p.event_type === eventType && p.channel === channel,
  )
  return pref?.enabled ?? true // opt-out model: default is enabled
}

// ── Page ──────────────────────────────────────────────────────────────────────

export default function NotificationPreferencesPage() {
  const params = useParams<{ orgSlug: string }>()
  const orgSlug = params.orgSlug
  const queryClient = useQueryClient()

  const { data, isLoading, isError, refetch } = useQuery({
    queryKey: notificationKeys.preferences(orgSlug),
    queryFn: () => notificationsApi.getPreferences(orgSlug).then((r) => r.data),
    staleTime: 60_000,
  })

  const preferences = data?.preferences ?? []

  const mutation = useMutation({
    mutationFn: ({
      eventType,
      channel,
      enabled,
    }: {
      eventType: NotificationEventType
      channel: NotificationChannel
      enabled: boolean
    }) => notificationsApi.updatePreference(orgSlug, eventType, { channel, enabled }),

    onMutate: async ({ eventType, channel, enabled }) => {
      await queryClient.cancelQueries({ queryKey: notificationKeys.preferences(orgSlug) })
      const prev = queryClient.getQueryData(notificationKeys.preferences(orgSlug))

      queryClient.setQueryData(
        notificationKeys.preferences(orgSlug),
        (old: { preferences: NotificationPreference[] } | undefined) => {
          if (!old) return old
          const exists = old.preferences.find(
            (p) => p.event_type === eventType && p.channel === channel,
          )
          if (exists) {
            return {
              ...old,
              preferences: old.preferences.map((p) =>
                p.event_type === eventType && p.channel === channel
                  ? { ...p, enabled }
                  : p,
              ),
            }
          }
          // Insert a synthetic preference record for the opt-out
          const syntheticPref: NotificationPreference = {
            id: `${eventType}-${channel}`,
            organization_id: "",
            user_id: "",
            event_type: eventType,
            channel,
            enabled,
            updated_at: new Date().toISOString(),
          }
          return { ...old, preferences: [...old.preferences, syntheticPref] }
        },
      )

      return { prev }
    },

    onError: (_err, _vars, ctx) => {
      if (ctx?.prev) {
        queryClient.setQueryData(notificationKeys.preferences(orgSlug), ctx.prev)
      }
      toast.error(extractApiError(_err) ?? "Failed to update preference")
    },

    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: notificationKeys.preferences(orgSlug) })
    },
  })

  return (
    <div className="space-y-6">
      <div>
        <h2 className="text-lg font-semibold">Notification preferences</h2>
        <p className="text-sm text-muted-foreground">
          Choose how you want to be notified for each event type.
        </p>
      </div>

      <Separator />

      {isError && (
        <div className="flex flex-col items-center gap-3 rounded-xl border border-dashed border-border py-12 text-center">
          <p className="text-sm text-muted-foreground">Failed to load preferences.</p>
          <Button variant="outline" size="sm" onClick={() => refetch()}>
            Retry
          </Button>
        </div>
      )}

      {isLoading ? (
        <PreferencesSkeleton />
      ) : (
        <div className="space-y-8">
          {/* Column headers — webhook channel is intentionally excluded: it is
              configured per-endpoint in the Webhooks section, not here. */}
          <div className="grid grid-cols-[1fr_auto_auto] items-center gap-4 text-xs font-medium text-muted-foreground">
            <span>Event</span>
            <span className="w-12 text-center">In-app</span>
            <span className="w-12 text-center">Email</span>
          </div>

          {GROUPS.map((group) => {
            const groupEvents = ALL_EVENT_TYPES.filter(
              (et) => EVENT_TYPE_INFO[et].group === group,
            )
            return (
              <section key={group} aria-labelledby={`group-${group}`}>
                <h3
                  id={`group-${group}`}
                  className="mb-3 text-xs font-semibold uppercase tracking-wider text-muted-foreground"
                >
                  {group}
                </h3>
                <div className="divide-y divide-border rounded-xl border border-border">
                  {groupEvents.map((eventType, i) => {
                    const info = EVENT_TYPE_INFO[eventType]
                    const inAppEnabled = getEffectiveEnabled(preferences, eventType, "in_app")
                    const emailEnabled = getEffectiveEnabled(preferences, eventType, "email")
                    const isPending = mutation.isPending && mutation.variables?.eventType === eventType

                    return (
                      <div
                        key={eventType}
                        className={`grid grid-cols-[1fr_auto_auto] items-center gap-4 px-4 py-3.5 ${
                          i === 0 ? "rounded-t-xl" : ""
                        } ${i === groupEvents.length - 1 ? "rounded-b-xl" : ""}`}
                      >
                        <div className="min-w-0">
                          <p className="text-sm font-medium">{info.label}</p>
                          <p className="text-xs text-muted-foreground">{info.description}</p>
                        </div>

                        {/* In-app toggle */}
                        <div className="flex w-12 items-center justify-center">
                          {isPending && mutation.variables?.channel === "in_app" ? (
                            <Loader2Icon className="size-4 animate-spin text-muted-foreground" />
                          ) : (
                            <Switch
                              checked={inAppEnabled}
                              onCheckedChange={(checked) =>
                                mutation.mutate({
                                  eventType,
                                  channel: "in_app",
                                  enabled: checked,
                                })
                              }
                              aria-label={`${info.label} in-app notifications`}
                              disabled={mutation.isPending}
                            />
                          )}
                        </div>

                        {/* Email toggle */}
                        <div className="flex w-12 items-center justify-center">
                          {isPending && mutation.variables?.channel === "email" ? (
                            <Loader2Icon className="size-4 animate-spin text-muted-foreground" />
                          ) : (
                            <Switch
                              checked={emailEnabled}
                              onCheckedChange={(checked) =>
                                mutation.mutate({
                                  eventType,
                                  channel: "email",
                                  enabled: checked,
                                })
                              }
                              aria-label={`${info.label} email notifications`}
                              disabled={mutation.isPending}
                            />
                          )}
                        </div>
                      </div>
                    )
                  })}
                </div>
              </section>
            )
          })}
        </div>
      )}
    </div>
  )
}

function PreferencesSkeleton() {
  return (
    <div className="space-y-8" aria-busy="true" aria-label="Loading preferences">
      {GROUPS.map((group) => (
        <div key={group} className="space-y-3">
          <Skeleton className="h-3 w-20" />
          <div className="rounded-xl border border-border divide-y divide-border">
            {Array.from({ length: group === "Matches" ? 5 : group === "Tournaments" ? 1 : 3 }).map(
              (_, i) => (
                <div
                  key={i}
                  className="grid grid-cols-[1fr_auto_auto] items-center gap-4 px-4 py-3.5"
                >
                  <div className="space-y-1.5">
                    <Skeleton className="h-3.5 w-32" />
                    <Skeleton className="h-3 w-48" />
                  </div>
                  <Skeleton className="h-5 w-9 rounded-full" />
                  <Skeleton className="h-5 w-9 rounded-full" />
                </div>
              ),
            )}
          </div>
        </div>
      ))}
    </div>
  )
}
