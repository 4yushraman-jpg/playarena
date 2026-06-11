"use client"

import Link from "next/link"
import { useParams, useRouter } from "next/navigation"
import {
  PencilIcon,
  Trash2Icon,
  CalendarIcon,
  UserIcon,
  RulerIcon,
  WeightIcon,
  FlagIcon,
  ShieldIcon,
} from "lucide-react"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { StatusBadge } from "@/components/ui/status-badge"
import { PageHeader } from "@/components/ui/page-header"
import { ConfirmDialog } from "@/components/ui/confirm-dialog"
import { PlayerAvatar } from "@/components/players/player-avatar"
import { usePlayer, useDeletePlayer } from "@/hooks/use-players"
import { useAuthStore, selectRole } from "@/stores/auth.store"
import { hasPermission } from "@/lib/permissions"
import { formatDate, formatRelative } from "@/lib/format"
import * as React from "react"

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

// ── Page ──────────────────────────────────────────────────────────────────────

export default function PlayerProfilePage() {
  const params = useParams<{ orgSlug: string; playerId: string }>()
  const { orgSlug, playerId } = params
  const router = useRouter()
  const role = useAuthStore(selectRole)
  const [confirmDelete, setConfirmDelete] = React.useState(false)

  const { data: player, isLoading, isError } = usePlayer(orgSlug, playerId)
  const deletePlayer = useDeletePlayer(orgSlug)

  const canEdit = hasPermission(role, "player.update")
  const canDelete = hasPermission(role, "player.delete")
  const canUploadMedia = hasPermission(role, "media.upload")

  function handleDelete() {
    deletePlayer.mutate(playerId, {
      onSuccess: () => router.push(`/${orgSlug}/players`),
    })
  }

  if (isLoading) {
    return <PlayerProfileSkeleton />
  }

  if (isError || !player) {
    return (
      <div className="flex flex-col items-center gap-3 rounded-xl border border-dashed border-border py-20 text-center">
        <p className="text-sm font-medium">Player not found</p>
        <Button asChild variant="outline" size="sm">
          <Link href={`/${orgSlug}/players`}>Back to Players</Link>
        </Button>
      </div>
    )
  }

  return (
    <div className="mx-auto max-w-3xl space-y-6">
      <PageHeader
        title={player.display_name}
        breadcrumbs={[
          { label: "Dashboard", href: `/${orgSlug}` },
          { label: "Players", href: `/${orgSlug}/players` },
          { label: player.display_name },
        ]}
        action={
          <div className="flex items-center gap-2">
            {canEdit && (
              <Button asChild size="sm" variant="outline" className="gap-1.5">
                <Link href={`/${orgSlug}/players/${playerId}/edit`}>
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
                Remove
              </Button>
            )}
          </div>
        }
      />

      <div className="grid gap-6 lg:grid-cols-3">
        {/* Left — avatar + identity */}
        <div className="lg:col-span-1">
          <div className="rounded-xl border border-border bg-card p-6">
            <div className="flex flex-col items-center gap-4 text-center">
              <PlayerAvatar
                orgSlug={orgSlug}
                playerId={playerId}
                avatarUrl={player.avatar_url}
                displayName={player.display_name}
                size="lg"
                canUpload={canUploadMedia}
              />
              <div className="space-y-1">
                <h2 className="text-lg font-semibold">{player.display_name}</h2>
                {player.position && (
                  <p className="text-sm text-muted-foreground">{player.position}</p>
                )}
              </div>
              <StatusBadge status={player.status} />
              {player.jersey_number && (
                <div className="flex items-center gap-1.5 text-2xl font-bold tabular-nums">
                  <span className="text-sm font-normal text-muted-foreground">#</span>
                  {player.jersey_number}
                </div>
              )}
            </div>
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
                icon={CalendarIcon}
                label="Date of birth"
                value={player.date_of_birth ? formatDate(player.date_of_birth) : "—"}
              />
              <DetailRow
                icon={FlagIcon}
                label="Nationality"
                value={player.nationality ?? "—"}
              />
              <DetailRow
                icon={RulerIcon}
                label="Height"
                value={player.height_cm ? `${player.height_cm} cm` : "—"}
              />
              <DetailRow
                icon={WeightIcon}
                label="Weight"
                value={player.weight_kg ? `${player.weight_kg} kg` : "—"}
              />
              <DetailRow
                icon={UserIcon}
                label="Dominant hand"
                value={player.dominant_hand ? capitalize(player.dominant_hand) : "—"}
              />
              <DetailRow
                icon={CalendarIcon}
                label="Added"
                value={formatRelative(player.created_at)}
              />
            </div>
          </div>

          {player.bio && (
            <div className="rounded-xl border border-border bg-card p-6">
              <h3 className="mb-2 text-sm font-semibold uppercase tracking-wide text-muted-foreground">
                Bio
              </h3>
              <p className="text-sm leading-relaxed text-foreground/80">{player.bio}</p>
            </div>
          )}

          {/* Team memberships placeholder — populated in FE-6 when memberships are cross-linked */}
          <div className="rounded-xl border border-border bg-card p-6">
            <h3 className="mb-2 text-sm font-semibold uppercase tracking-wide text-muted-foreground">
              Teams
            </h3>
            <div className="flex items-center gap-2 text-sm text-muted-foreground">
              <ShieldIcon className="size-4" />
              <span>Team memberships are managed from the team profile.</span>
            </div>
          </div>
        </div>
      </div>

      <ConfirmDialog
        open={confirmDelete}
        onOpenChange={setConfirmDelete}
        title="Remove player"
        description={`Remove ${player.display_name} from the organization? This action marks them as inactive and cannot be undone.`}
        confirmLabel="Remove"
        destructive
        isPending={deletePlayer.isPending}
        onConfirm={handleDelete}
      />
    </div>
  )
}

function capitalize(s: string) {
  return s.charAt(0).toUpperCase() + s.slice(1)
}

function PlayerProfileSkeleton() {
  return (
    <div className="mx-auto max-w-3xl space-y-6" aria-busy="true" aria-label="Loading player">
      <div className="space-y-2">
        <Skeleton className="h-4 w-40" />
        <Skeleton className="h-8 w-56" />
      </div>
      <div className="grid gap-6 lg:grid-cols-3">
        <div className="rounded-xl border border-border p-6 space-y-4">
          <Skeleton className="size-20 mx-auto rounded-full" />
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
