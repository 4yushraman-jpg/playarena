"use client"

import { useParams, useRouter } from "next/navigation"
import { PageHeader } from "@/components/ui/page-header"
import { PlayerForm, coercePlayerFormValues, type PlayerFormValues } from "@/components/players/player-form"
import { useCreatePlayer } from "@/hooks/use-players"
import { useAuthStore, selectRole } from "@/stores/auth.store"
import { hasPermission } from "@/lib/permissions"

export default function NewPlayerPage() {
  const params = useParams<{ orgSlug: string }>()
  const orgSlug = params.orgSlug
  const router = useRouter()
  const role = useAuthStore(selectRole)

  const canCreate = hasPermission(role, "player.create")
  const createPlayer = useCreatePlayer(orgSlug)

  if (!canCreate) {
    return (
      <div className="rounded-xl border border-dashed border-border p-12 text-center">
        <p className="text-sm font-medium">You don&apos;t have permission to create players.</p>
      </div>
    )
  }

  function handleSubmit(values: PlayerFormValues) {
    createPlayer.mutate(coercePlayerFormValues(values), {
      onSuccess: (response) => {
        router.push(`/${orgSlug}/players/${response.data.id}`)
      },
    })
  }

  return (
    <div className="mx-auto max-w-2xl space-y-6">
      <PageHeader
        title="New player"
        description="Add a player to your organization's roster."
        breadcrumbs={[
          { label: "Dashboard", href: `/${orgSlug}` },
          { label: "Players", href: `/${orgSlug}/players` },
          { label: "New player" },
        ]}
      />

      <div className="rounded-xl border border-border bg-card p-6">
        <PlayerForm
          isPending={createPlayer.isPending}
          onSubmit={handleSubmit}
          onCancel={() => router.push(`/${orgSlug}/players`)}
        />
      </div>
    </div>
  )
}
