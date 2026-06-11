"use client"

import { useParams, useRouter } from "next/navigation"
import { PageHeader } from "@/components/ui/page-header"
import {
  TournamentForm,
  coerceTournamentFormValues,
  type TournamentFormValues,
} from "@/components/tournaments/tournament-form"
import { useCreateTournament } from "@/hooks/use-tournaments"
import { useAuthStore, selectRole } from "@/stores/auth.store"
import { hasPermission } from "@/lib/permissions"

export default function NewTournamentPage() {
  const params = useParams<{ orgSlug: string }>()
  const orgSlug = params.orgSlug
  const router = useRouter()
  const role = useAuthStore(selectRole)
  const canCreate = hasPermission(role, "tournament.create")
  const createTournament = useCreateTournament(orgSlug)

  if (!canCreate) {
    return (
      <div className="rounded-xl border border-dashed border-border p-12 text-center">
        <p className="text-sm font-medium">
          You don&apos;t have permission to create tournaments.
        </p>
      </div>
    )
  }

  function handleSubmit(values: TournamentFormValues) {
    createTournament.mutate(coerceTournamentFormValues(values), {
      onSuccess: (response) => {
        router.push(`/${orgSlug}/tournaments/${response.data.id}`)
      },
    })
  }

  return (
    <div className="mx-auto max-w-2xl space-y-6">
      <PageHeader
        title="New tournament"
        description="Create a tournament and start accepting team registrations."
        breadcrumbs={[
          { label: "Dashboard", href: `/${orgSlug}` },
          { label: "Tournaments", href: `/${orgSlug}/tournaments` },
          { label: "New tournament" },
        ]}
      />

      <div className="rounded-xl border border-border bg-card p-6">
        <TournamentForm
          isPending={createTournament.isPending}
          onSubmit={handleSubmit}
          onCancel={() => router.push(`/${orgSlug}/tournaments`)}
        />
      </div>
    </div>
  )
}
