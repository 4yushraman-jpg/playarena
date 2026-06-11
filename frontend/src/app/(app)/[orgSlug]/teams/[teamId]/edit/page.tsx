"use client"

import Link from "next/link"
import { useParams, useRouter } from "next/navigation"
import { PageHeader } from "@/components/ui/page-header"
import { Skeleton } from "@/components/ui/skeleton"
import { Button } from "@/components/ui/button"
import { TeamForm, coerceTeamFormValues, type TeamFormValues } from "@/components/teams/team-form"
import { TeamLogo } from "@/components/teams/team-logo"
import { useTeam, useUpdateTeam } from "@/hooks/use-teams"
import { useAuthStore, selectRole } from "@/stores/auth.store"
import { hasPermission } from "@/lib/permissions"

export default function EditTeamPage() {
  const params = useParams<{ orgSlug: string; teamId: string }>()
  const { orgSlug, teamId } = params
  const router = useRouter()
  const role = useAuthStore(selectRole)

  const canEdit = hasPermission(role, "team.update")
  const canUploadMedia = hasPermission(role, "media.upload")

  const { data: team, isLoading, isError } = useTeam(orgSlug, teamId)
  const updateTeam = useUpdateTeam(orgSlug, teamId)

  if (!canEdit) {
    return (
      <div className="rounded-xl border border-dashed border-border p-12 text-center">
        <p className="text-sm font-medium">You don&apos;t have permission to edit teams.</p>
      </div>
    )
  }

  if (isLoading) {
    return (
      <div className="mx-auto max-w-2xl space-y-6" aria-busy="true">
        <div className="space-y-2">
          <Skeleton className="h-4 w-48" />
          <Skeleton className="h-8 w-32" />
        </div>
        <div className="rounded-xl border border-border p-6 space-y-5">
          {Array.from({ length: 5 }).map((_, i) => (
            <div key={i} className="space-y-1.5">
              <Skeleton className="h-4 w-24" />
              <Skeleton className="h-9 w-full" />
            </div>
          ))}
        </div>
      </div>
    )
  }

  if (isError || !team) {
    return (
      <div className="flex flex-col items-center gap-3 rounded-xl border border-dashed border-border py-20 text-center">
        <p className="text-sm font-medium">Team not found</p>
        <Button asChild variant="outline" size="sm">
          <Link href={`/${orgSlug}/teams`}>Back to Teams</Link>
        </Button>
      </div>
    )
  }

  function handleSubmit(values: TeamFormValues) {
    const coerced = coerceTeamFormValues(values)
    updateTeam.mutate(
      {
        name: coerced.name,
        short_name: coerced.short_name ?? null,
        description: coerced.description ?? null,
        home_city: coerced.home_city ?? null,
        home_venue: coerced.home_venue ?? null,
        founded_year: coerced.founded_year ?? null,
        primary_color: coerced.primary_color ?? null,
        secondary_color: coerced.secondary_color ?? null,
        status: values.status,
      },
      {
        onSuccess: () => router.push(`/${orgSlug}/teams/${teamId}`),
      },
    )
  }

  return (
    <div className="mx-auto max-w-2xl space-y-6">
      <PageHeader
        title="Edit team"
        breadcrumbs={[
          { label: "Dashboard", href: `/${orgSlug}` },
          { label: "Teams", href: `/${orgSlug}/teams` },
          { label: team.name, href: `/${orgSlug}/teams/${teamId}` },
          { label: "Edit" },
        ]}
      />

      {/* Logo section */}
      <div className="flex items-center gap-4 rounded-xl border border-border bg-card p-4">
        <TeamLogo
          orgSlug={orgSlug}
          teamId={teamId}
          logoUrl={team.logo_url}
          teamName={team.name}
          primaryColor={team.primary_color}
          size="md"
          canUpload={canUploadMedia}
        />
        <div>
          <p className="text-sm font-medium">{team.name}</p>
          <p className="text-xs text-muted-foreground">
            {canUploadMedia
              ? "Click the camera icon to upload a team logo."
              : "Logo upload requires media permission."}
          </p>
        </div>
      </div>

      <div className="rounded-xl border border-border bg-card p-6">
        <TeamForm
          defaultValues={team}
          isEdit
          isPending={updateTeam.isPending}
          onSubmit={handleSubmit}
          onCancel={() => router.push(`/${orgSlug}/teams/${teamId}`)}
        />
      </div>
    </div>
  )
}
