"use client"

import { useParams, useRouter } from "next/navigation"
import { PageHeader } from "@/components/ui/page-header"
import { TeamForm, coerceTeamFormValues, type TeamFormValues } from "@/components/teams/team-form"
import { useCreateTeam } from "@/hooks/use-teams"
import { useAuthStore, selectRole } from "@/stores/auth.store"
import { hasPermission } from "@/lib/permissions"

export default function NewTeamPage() {
  const params = useParams<{ orgSlug: string }>()
  const orgSlug = params.orgSlug
  const router = useRouter()
  const role = useAuthStore(selectRole)

  const canCreate = hasPermission(role, "team.create")
  const createTeam = useCreateTeam(orgSlug)

  if (!canCreate) {
    return (
      <div className="rounded-xl border border-dashed border-border p-12 text-center">
        <p className="text-sm font-medium">You don&apos;t have permission to create teams.</p>
      </div>
    )
  }

  function handleSubmit(values: TeamFormValues) {
    createTeam.mutate(coerceTeamFormValues(values), {
      onSuccess: (response) => {
        router.push(`/${orgSlug}/teams/${response.data.id}`)
      },
    })
  }

  return (
    <div className="mx-auto max-w-2xl space-y-6">
      <PageHeader
        title="New team"
        description="Create a team and start building your roster."
        breadcrumbs={[
          { label: "Dashboard", href: `/${orgSlug}` },
          { label: "Teams", href: `/${orgSlug}/teams` },
          { label: "New team" },
        ]}
      />

      <div className="rounded-xl border border-border bg-card p-6">
        <TeamForm
          isPending={createTeam.isPending}
          onSubmit={handleSubmit}
          onCancel={() => router.push(`/${orgSlug}/teams`)}
        />
      </div>
    </div>
  )
}
