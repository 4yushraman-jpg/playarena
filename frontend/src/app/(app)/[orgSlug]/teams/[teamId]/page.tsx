"use client"

import * as React from "react"
import Link from "next/link"
import { useParams, useRouter } from "next/navigation"
import {
  PencilIcon,
  Trash2Icon,
  MapPinIcon,
  CalendarIcon,
  PaletteIcon,
  BuildingIcon,
} from "lucide-react"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { StatusBadge } from "@/components/ui/status-badge"
import { PageHeader } from "@/components/ui/page-header"
import { ConfirmDialog } from "@/components/ui/confirm-dialog"
import { TeamLogo } from "@/components/teams/team-logo"
import { MembersSection } from "@/components/teams/members-section"
import { useTeam, useDeleteTeam } from "@/hooks/use-teams"
import { useAuthStore, selectRole } from "@/stores/auth.store"
import { hasPermission } from "@/lib/permissions"
import { formatRelative } from "@/lib/format"

// ── Detail row ────────────────────────────────────────────────────────────────

function DetailRow({
  icon: Icon,
  label,
  value,
}: {
  icon: React.ElementType
  label: string
  value: React.ReactNode
}) {
  return (
    <div className="flex items-start gap-3">
      <div className="mt-0.5 flex size-7 shrink-0 items-center justify-center rounded-md bg-muted text-muted-foreground">
        <Icon className="size-3.5" />
      </div>
      <div className="min-w-0 flex-1">
        <p className="text-xs text-muted-foreground">{label}</p>
        <p className="text-sm font-medium">{value}</p>
      </div>
    </div>
  )
}

// ── Color swatch ──────────────────────────────────────────────────────────────

function ColorSwatch({ color, label }: { color: string; label: string }) {
  return (
    <div className="flex items-center gap-2">
      <div
        className="size-4 rounded border border-border"
        style={{ backgroundColor: color }}
        aria-hidden="true"
      />
      <span className="text-sm font-mono">{color}</span>
      <span className="text-xs text-muted-foreground">{label}</span>
    </div>
  )
}

// ── Page ──────────────────────────────────────────────────────────────────────

export default function TeamProfilePage() {
  const params = useParams<{ orgSlug: string; teamId: string }>()
  const { orgSlug, teamId } = params
  const router = useRouter()
  const role = useAuthStore(selectRole)
  const [confirmDelete, setConfirmDelete] = React.useState(false)

  const { data: team, isLoading, isError } = useTeam(orgSlug, teamId)
  const deleteTeam = useDeleteTeam(orgSlug)

  const canEdit = hasPermission(role, "team.update")
  const canDelete = hasPermission(role, "team.delete")
  const canManageMembers = hasPermission(role, "team.update")
  const canUploadMedia = hasPermission(role, "media.upload")

  function handleDelete() {
    deleteTeam.mutate(teamId, {
      onSuccess: () => router.push(`/${orgSlug}/teams`),
    })
  }

  if (isLoading) {
    return <TeamProfileSkeleton />
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

  return (
    <div className="mx-auto max-w-3xl space-y-6">
      <PageHeader
        title={team.name}
        breadcrumbs={[
          { label: "Dashboard", href: `/${orgSlug}` },
          { label: "Teams", href: `/${orgSlug}/teams` },
          { label: team.name },
        ]}
        action={
          <div className="flex items-center gap-2">
            {canEdit && (
              <Button asChild size="sm" variant="outline" className="gap-1.5">
                <Link href={`/${orgSlug}/teams/${teamId}/edit`}>
                  <PencilIcon className="size-3.5" />
                  Edit
                </Link>
              </Button>
            )}
            {canDelete && (
              <Button
                size="sm"
                variant="outline"
                className="gap-1.5 text-destructive hover:bg-destructive/10"
                onClick={() => setConfirmDelete(true)}
              >
                <Trash2Icon className="size-3.5" />
                Disband
              </Button>
            )}
          </div>
        }
      />

      <div className="grid gap-6 lg:grid-cols-3">
        {/* Left — logo + identity */}
        <div className="lg:col-span-1">
          <div className="rounded-xl border border-border bg-card p-6">
            <div className="flex flex-col items-center gap-4 text-center">
              <TeamLogo
                orgSlug={orgSlug}
                teamId={teamId}
                logoUrl={team.logo_url}
                teamName={team.name}
                primaryColor={team.primary_color}
                size="lg"
                canUpload={canUploadMedia}
              />
              <div className="space-y-1">
                <h2 className="text-lg font-semibold">{team.name}</h2>
                {team.short_name && (
                  <p className="text-sm text-muted-foreground font-mono">{team.short_name}</p>
                )}
              </div>
              <StatusBadge status={team.status} />
            </div>

            {(team.primary_color || team.secondary_color) && (
              <div className="mt-4 space-y-2 border-t border-border pt-4">
                {team.primary_color && (
                  <ColorSwatch color={team.primary_color} label="Primary" />
                )}
                {team.secondary_color && (
                  <ColorSwatch color={team.secondary_color} label="Secondary" />
                )}
              </div>
            )}
          </div>
        </div>

        {/* Right — details */}
        <div className="lg:col-span-2 space-y-4">
          <div className="rounded-xl border border-border bg-card p-6">
            <h3 className="mb-4 text-sm font-semibold uppercase tracking-wide text-muted-foreground">
              Details
            </h3>
            <div className="grid gap-4 sm:grid-cols-2">
              <DetailRow
                icon={MapPinIcon}
                label="Home city"
                value={team.home_city ?? "—"}
              />
              <DetailRow
                icon={BuildingIcon}
                label="Home venue"
                value={team.home_venue ?? "—"}
              />
              <DetailRow
                icon={CalendarIcon}
                label="Founded"
                value={team.founded_year ?? "—"}
              />
              <DetailRow
                icon={PaletteIcon}
                label="Added"
                value={formatRelative(team.created_at)}
              />
            </div>
          </div>

          {team.description && (
            <div className="rounded-xl border border-border bg-card p-6">
              <h3 className="mb-2 text-sm font-semibold uppercase tracking-wide text-muted-foreground">
                About
              </h3>
              <p className="text-sm leading-relaxed text-foreground/80">{team.description}</p>
            </div>
          )}
        </div>
      </div>

      {/* Members section — full width */}
      <div className="rounded-xl border border-border bg-card p-6">
        <MembersSection
          orgSlug={orgSlug}
          teamId={teamId}
          canManage={canManageMembers}
        />
      </div>

      <ConfirmDialog
        open={confirmDelete}
        onOpenChange={setConfirmDelete}
        title="Disband team"
        description={`Disband ${team.name}? The team will be marked as disbanded and removed from active listings.`}
        confirmLabel="Disband"
        destructive
        isPending={deleteTeam.isPending}
        onConfirm={handleDelete}
      />
    </div>
  )
}

function TeamProfileSkeleton() {
  return (
    <div className="mx-auto max-w-3xl space-y-6" aria-busy="true" aria-label="Loading team">
      <div className="space-y-2">
        <Skeleton className="h-4 w-40" />
        <Skeleton className="h-8 w-48" />
      </div>
      <div className="grid gap-6 lg:grid-cols-3">
        <div className="rounded-xl border border-border p-6 space-y-4">
          <Skeleton className="size-20 mx-auto rounded-xl" />
          <div className="space-y-2 text-center">
            <Skeleton className="h-5 w-32 mx-auto" />
            <Skeleton className="h-4 w-20 mx-auto" />
          </div>
        </div>
        <div className="lg:col-span-2 rounded-xl border border-border p-6 space-y-4">
          {Array.from({ length: 4 }).map((_, i) => (
            <div key={i} className="flex gap-3">
              <Skeleton className="size-7 rounded-md shrink-0" />
              <div className="flex-1 space-y-1">
                <Skeleton className="h-3 w-16" />
                <Skeleton className="h-4 w-24" />
              </div>
            </div>
          ))}
        </div>
      </div>
    </div>
  )
}
