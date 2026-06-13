"use client"

import { useMemo } from "react"
import Link from "next/link"
import { useParams, useRouter } from "next/navigation"
import { Loader2Icon } from "lucide-react"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { PageHeader } from "@/components/ui/page-header"
import {
  FixtureForm,
  coerceFixtureValues,
  rfc3339ToLocal,
  type FixtureFormValues,
  type FixtureParticipant,
} from "@/components/matches/fixture-form"
import { registrationsToParticipants, buildUpdateMatchBody } from "@/components/matches/fixture-mapping"
import { useMatch, useUpdateMatch } from "@/hooks/use-matches"
import { useRegistrationList } from "@/hooks/use-registrations"
import { useParticipantNames } from "@/hooks/use-participant-names"
import { useAuthStore, selectRole } from "@/stores/auth.store"
import { hasPermission } from "@/lib/permissions"
import { isFixtureEditable, matchParticipantIds, matchParticipantType } from "@/lib/match-meta"

const PICKER_LIMIT = 100

export default function EditMatchPage() {
  const params = useParams<{ orgSlug: string; matchId: string }>()
  const { orgSlug, matchId } = params
  const router = useRouter()
  const role = useAuthStore(selectRole)
  const canUpdate = hasPermission(role, "match.update")

  const { data: match, isLoading, isError } = useMatch(orgSlug, matchId)
  const { resolve } = useParticipantNames(orgSlug)

  const isTeam = match ? matchParticipantType(match) === "team" : true
  const registrationsQuery = useRegistrationList(
    orgSlug,
    match?.tournament_id ?? "",
    { status: "approved", limit: PICKER_LIMIT },
    { enabled: !!match },
  )

  const updateMatch = useUpdateMatch(orgSlug, matchId)

  // Build the participant option list from approved registrants, then make sure
  // the fixture's CURRENT participants are present even if their registration
  // later changed status — otherwise the prefilled select would show blank.
  const participants: FixtureParticipant[] = useMemo(() => {
    if (!match) return []
    const list = registrationsToParticipants(
      registrationsQuery.data?.registrations ?? [],
      isTeam,
    )

    const seen = new Set(list.map((p) => p.id))
    const { homeId, awayId } = matchParticipantIds(match)
    for (const id of [homeId, awayId]) {
      if (id && !seen.has(id)) {
        list.push({ id, name: resolve(isTeam ? id : null, isTeam ? null : id) })
        seen.add(id)
      }
    }
    return list
  }, [match, registrationsQuery.data, isTeam, resolve])

  const defaultValues: Partial<FixtureFormValues> | undefined = useMemo(() => {
    if (!match) return undefined
    const { homeId, awayId } = matchParticipantIds(match)
    return {
      home_participant_id: homeId ?? "",
      away_participant_id: awayId ?? "",
      scheduled_at: rfc3339ToLocal(match.scheduled_at),
      venue: match.venue ?? "",
      round_name: match.round_name ?? "",
      round_number: match.round_number != null ? String(match.round_number) : "",
      match_number: match.match_number != null ? String(match.match_number) : "",
    }
  }, [match])

  if (isLoading) {
    return (
      <div className="mx-auto max-w-2xl space-y-6" aria-busy="true" aria-label="Loading fixture">
        <Skeleton className="h-8 w-48" />
        <Skeleton className="h-96 w-full rounded-xl" />
      </div>
    )
  }

  if (isError || !match) {
    return (
      <NoticeCard
        title="Match not found"
        body="This match may have been removed or you may not have access."
        href={`/${orgSlug}/matches`}
        linkLabel="Back to Matches"
      />
    )
  }

  if (!canUpdate) {
    return (
      <NoticeCard
        title="You don't have permission to edit fixtures."
        href={`/${orgSlug}/matches/${matchId}`}
        linkLabel="Back to match"
      />
    )
  }

  // Only scheduled fixtures are editable; live/terminal matches are owned by the
  // live-scoring surface.
  if (!isFixtureEditable(match)) {
    return (
      <NoticeCard
        title="This fixture can no longer be edited."
        body="Only scheduled matches can be rescheduled or reassigned."
        href={`/${orgSlug}/matches/${matchId}`}
        linkLabel="Back to match"
      />
    )
  }

  function handleSubmit(values: FixtureFormValues) {
    const body = buildUpdateMatchBody(isTeam, coerceFixtureValues(values))
    updateMatch.mutate(body, {
      onSuccess: () => router.push(`/${orgSlug}/matches/${matchId}`),
    })
  }

  return (
    <div className="mx-auto max-w-2xl space-y-6">
      <PageHeader
        title="Edit fixture"
        description="Reschedule or reassign this scheduled match."
        breadcrumbs={[
          { label: "Dashboard", href: `/${orgSlug}` },
          { label: "Matches", href: `/${orgSlug}/matches` },
          { label: "Edit fixture" },
        ]}
      />

      <div className="rounded-xl border border-border bg-card p-6">
        {registrationsQuery.isLoading ? (
          <div className="flex items-center gap-2 py-8 text-sm text-muted-foreground">
            <Loader2Icon className="size-4 animate-spin" />
            Loading participants…
          </div>
        ) : (
          <FixtureForm
            participants={participants}
            participantNoun={isTeam ? "team" : "player"}
            isEdit
            isPending={updateMatch.isPending}
            defaultValues={defaultValues}
            onSubmit={handleSubmit}
            onCancel={() => router.push(`/${orgSlug}/matches/${matchId}`)}
          />
        )}
      </div>
    </div>
  )
}

function NoticeCard({
  title,
  body,
  href,
  linkLabel,
}: {
  title: string
  body?: string
  href: string
  linkLabel: string
}) {
  return (
    <div className="mx-auto max-w-2xl rounded-xl border border-dashed border-border p-12 text-center">
      <p className="text-sm font-medium">{title}</p>
      {body && <p className="mt-1 text-xs text-muted-foreground">{body}</p>}
      <Button asChild variant="outline" size="sm" className="mt-4">
        <Link href={href}>{linkLabel}</Link>
      </Button>
    </div>
  )
}
