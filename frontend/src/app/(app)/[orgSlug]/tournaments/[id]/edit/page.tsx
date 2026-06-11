"use client"

import Link from "next/link"
import { useParams, useRouter } from "next/navigation"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { PageHeader } from "@/components/ui/page-header"
import {
  TournamentForm,
  coerceTournamentFormValues,
  type TournamentFormValues,
} from "@/components/tournaments/tournament-form"
import { useTournament, useUpdateTournament } from "@/hooks/use-tournaments"
import { useAuthStore, selectRole } from "@/stores/auth.store"
import { hasPermission } from "@/lib/permissions"
import { toast } from "sonner"

export default function EditTournamentPage() {
  const params = useParams<{ orgSlug: string; id: string }>()
  const { orgSlug, id } = params
  const router = useRouter()
  const role = useAuthStore(selectRole)
  const canEdit = hasPermission(role, "tournament.update")

  const { data: tournament, isLoading, isError } = useTournament(orgSlug, id)
  const updateTournament = useUpdateTournament(orgSlug, id)

  if (!canEdit) {
    return (
      <div className="rounded-xl border border-dashed border-border p-12 text-center">
        <p className="text-sm font-medium">
          You don&apos;t have permission to edit tournaments.
        </p>
      </div>
    )
  }

  if (isLoading) {
    return (
      <div className="mx-auto max-w-2xl space-y-6" aria-busy="true">
        <div className="space-y-2">
          <Skeleton className="h-4 w-64" />
          <Skeleton className="h-8 w-40" />
        </div>
        <div className="rounded-xl border border-border bg-card p-6 space-y-5">
          {Array.from({ length: 6 }).map((_, i) => (
            <div key={i} className="space-y-1.5">
              <Skeleton className="h-4 w-24" />
              <Skeleton className="h-9 w-full" />
            </div>
          ))}
        </div>
      </div>
    )
  }

  if (isError || !tournament) {
    return (
      <div className="flex flex-col items-center gap-3 rounded-xl border border-dashed border-border py-20 text-center">
        <p className="text-sm font-medium">Tournament not found</p>
        <Button asChild variant="outline" size="sm">
          <Link href={`/${orgSlug}/tournaments`}>Back to Tournaments</Link>
        </Button>
      </div>
    )
  }

  // Cancelled and completed tournaments are view-only
  if (tournament.status === "cancelled" || tournament.status === "completed") {
    return (
      <div className="rounded-xl border border-dashed border-border p-12 text-center">
        <p className="text-sm font-medium">
          This tournament is {tournament.status} and cannot be edited.
        </p>
        <Button asChild variant="outline" size="sm" className="mt-4">
          <Link href={`/${orgSlug}/tournaments/${id}`}>View tournament</Link>
        </Button>
      </div>
    )
  }

  function handleSubmit(values: TournamentFormValues) {
    const coerced = coerceTournamentFormValues(values)
    updateTournament.mutate(
      {
        name: coerced.name,
        sport: coerced.sport,
        format: coerced.format,
        participant_type: coerced.participant_type,
        description: coerced.description ?? null,
        prize_pool: coerced.prize_pool ?? null,
        currency: coerced.currency,
        max_participants: coerced.max_participants ?? null,
        min_participants: coerced.min_participants ?? null,
        registration_opens_at: coerced.registration_opens_at ?? null,
        registration_closes_at: coerced.registration_closes_at ?? null,
        starts_at: coerced.starts_at ?? null,
        ends_at: coerced.ends_at ?? null,
        venue: coerced.venue ?? null,
        city: coerced.city ?? null,
        country: coerced.country ?? null,
        rules: coerced.rules ?? null,
      },
      {
        onSuccess: () => {
          toast.success("Tournament updated")
          router.push(`/${orgSlug}/tournaments/${id}`)
        },
      },
    )
  }

  return (
    <div className="mx-auto max-w-2xl space-y-6">
      <PageHeader
        title="Edit tournament"
        breadcrumbs={[
          { label: "Dashboard", href: `/${orgSlug}` },
          { label: "Tournaments", href: `/${orgSlug}/tournaments` },
          { label: tournament.name, href: `/${orgSlug}/tournaments/${id}` },
          { label: "Edit" },
        ]}
      />

      <div className="rounded-xl border border-border bg-card p-6">
        <TournamentForm
          defaultValues={tournament}
          isEdit
          isPending={updateTournament.isPending}
          onSubmit={handleSubmit}
          onCancel={() => router.push(`/${orgSlug}/tournaments/${id}`)}
        />
      </div>
    </div>
  )
}
