"use client"

import Link from "next/link"
import { useParams, useRouter } from "next/navigation"
import { PageHeader } from "@/components/ui/page-header"
import { Skeleton } from "@/components/ui/skeleton"
import { Button } from "@/components/ui/button"
import { PlayerForm, coercePlayerFormValues, type PlayerFormValues } from "@/components/players/player-form"
import { PlayerAvatar } from "@/components/players/player-avatar"
import { usePlayer, useUpdatePlayer } from "@/hooks/use-players"
import { useAuthStore, selectRole } from "@/stores/auth.store"
import { hasPermission } from "@/lib/permissions"

export default function EditPlayerPage() {
  const params = useParams<{ orgSlug: string; playerId: string }>()
  const { orgSlug, playerId } = params
  const router = useRouter()
  const role = useAuthStore(selectRole)

  const canEdit = hasPermission(role, "player.update")
  const canUploadMedia = hasPermission(role, "media.upload")

  const { data: player, isLoading, isError } = usePlayer(orgSlug, playerId)
  const updatePlayer = useUpdatePlayer(orgSlug, playerId)

  if (!canEdit) {
    return (
      <div className="rounded-xl border border-dashed border-border p-12 text-center">
        <p className="text-sm font-medium">You don&apos;t have permission to edit players.</p>
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

  function handleSubmit(values: PlayerFormValues) {
    const coerced = coercePlayerFormValues(values)
    updatePlayer.mutate(
      {
        display_name: coerced.display_name,
        jersey_number: coerced.jersey_number ?? null,
        position: coerced.position ?? null,
        height_cm: coerced.height_cm ?? null,
        weight_kg: coerced.weight_kg ?? null,
        dominant_hand: coerced.dominant_hand ?? null,
        date_of_birth: coerced.date_of_birth ?? null,
        nationality: coerced.nationality ?? null,
        bio: coerced.bio ?? null,
        status: values.status,
      },
      {
        onSuccess: () => router.push(`/${orgSlug}/players/${playerId}`),
      },
    )
  }

  return (
    <div className="mx-auto max-w-2xl space-y-6">
      <PageHeader
        title="Edit player"
        breadcrumbs={[
          { label: "Dashboard", href: `/${orgSlug}` },
          { label: "Players", href: `/${orgSlug}/players` },
          { label: player.display_name, href: `/${orgSlug}/players/${playerId}` },
          { label: "Edit" },
        ]}
      />

      {/* Avatar section */}
      <div className="flex items-center gap-4 rounded-xl border border-border bg-card p-4">
        <PlayerAvatar
          orgSlug={orgSlug}
          playerId={playerId}
          avatarUrl={player.avatar_url}
          displayName={player.display_name}
          size="md"
          canUpload={canUploadMedia}
        />
        <div>
          <p className="text-sm font-medium">{player.display_name}</p>
          <p className="text-xs text-muted-foreground">
            {canUploadMedia ? "Click the camera icon to upload an avatar." : "Avatar upload requires media permission."}
          </p>
        </div>
      </div>

      <div className="rounded-xl border border-border bg-card p-6">
        <PlayerForm
          defaultValues={player}
          isEdit
          isPending={updatePlayer.isPending}
          onSubmit={handleSubmit}
          onCancel={() => router.push(`/${orgSlug}/players/${playerId}`)}
        />
      </div>
    </div>
  )
}
