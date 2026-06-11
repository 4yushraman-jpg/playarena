"use client"

import * as React from "react"
import { PlusIcon, Trash2Icon, UsersIcon } from "lucide-react"
import { Button } from "@/components/ui/button"
import { EmptyState } from "@/components/ui/empty-state"
import { StatusBadge } from "@/components/ui/status-badge"
import { ConfirmDialog } from "@/components/ui/confirm-dialog"
import { Skeleton } from "@/components/ui/skeleton"
import { AddMemberDialog } from "@/components/teams/add-member-dialog"
import { AvatarDisplay } from "@/components/players/player-avatar"
import { useTeamMembers, useRemoveTeamMember } from "@/hooks/use-team-members"
import { formatDate } from "@/lib/format"

interface MembersSectionProps {
  orgSlug: string
  teamId: string
  canManage: boolean
}

export function MembersSection({ orgSlug, teamId, canManage }: MembersSectionProps) {
  const [addOpen, setAddOpen] = React.useState(false)
  const [removeTarget, setRemoveTarget] = React.useState<{ id: string; name: string } | null>(null)

  const { data, isLoading, isError, refetch } = useTeamMembers(orgSlug, teamId)
  const removeMember = useRemoveTeamMember(orgSlug, teamId)

  function handleRemoveConfirm() {
    if (!removeTarget) return
    removeMember.mutate(removeTarget.id, {
      onSuccess: () => setRemoveTarget(null),
    })
  }

  return (
    <section aria-labelledby="members-heading">
      <div className="mb-4 flex items-center justify-between gap-4">
        <div>
          <h2 id="members-heading" className="text-base font-semibold">
            Roster
          </h2>
          {data && (
            <p className="text-sm text-muted-foreground">
              {data.members.length} {data.members.length === 1 ? "member" : "members"}
            </p>
          )}
        </div>
        {canManage && (
          <Button
            size="sm"
            variant="outline"
            onClick={() => setAddOpen(true)}
            className="gap-1.5"
          >
            <PlusIcon className="size-3.5" />
            Add member
          </Button>
        )}
      </div>

      {isLoading ? (
        <MembersSkeleton />
      ) : isError ? (
        <div className="flex flex-col items-center gap-2 rounded-lg border border-dashed border-border py-8 text-center">
          <p className="text-sm text-muted-foreground">Failed to load roster</p>
          <Button variant="outline" size="sm" onClick={() => refetch()}>
            Retry
          </Button>
        </div>
      ) : !data?.members.length ? (
        <EmptyState
          icon={<UsersIcon />}
          title="No members yet"
          description={canManage ? "Add players to build the team roster." : "No players have been added yet."}
          action={
            canManage ? (
              <Button size="sm" onClick={() => setAddOpen(true)} className="gap-1.5">
                <PlusIcon className="size-3.5" />
                Add first member
              </Button>
            ) : undefined
          }
        />
      ) : (
        <div className="overflow-hidden rounded-lg border border-border">
          <table className="w-full text-sm" role="table" aria-label="Team roster">
            <thead>
              <tr className="border-b border-border bg-muted/40">
                <th scope="col" className="px-4 py-2.5 text-left font-medium text-muted-foreground">Player</th>
                <th scope="col" className="hidden px-4 py-2.5 text-left font-medium text-muted-foreground sm:table-cell">Role</th>
                <th scope="col" className="hidden px-4 py-2.5 text-left font-medium text-muted-foreground md:table-cell">Joined</th>
                <th scope="col" className="px-4 py-2.5 text-left font-medium text-muted-foreground">Status</th>
                {canManage && (
                  <th scope="col" className="px-4 py-2.5 text-right font-medium text-muted-foreground">
                    <span className="sr-only">Actions</span>
                  </th>
                )}
              </tr>
            </thead>
            <tbody className="divide-y divide-border">
              {data.members.map((member) => (
                <tr key={member.id} className="transition-colors hover:bg-muted/20">
                  <td className="px-4 py-3">
                    <div className="flex items-center gap-2.5">
                      <AvatarDisplay
                        displayName={member.player_display_name}
                        size="sm"
                      />
                      <div className="min-w-0">
                        <p className="truncate font-medium">{member.player_display_name}</p>
                        {member.jersey_number && (
                          <p className="text-xs text-muted-foreground">#{member.jersey_number}</p>
                        )}
                      </div>
                    </div>
                  </td>
                  <td className="hidden px-4 py-3 text-muted-foreground sm:table-cell capitalize">
                    {member.role || "—"}
                  </td>
                  <td className="hidden px-4 py-3 text-muted-foreground md:table-cell">
                    {formatDate(member.joined_at)}
                  </td>
                  <td className="px-4 py-3">
                    <StatusBadge status={member.status} />
                  </td>
                  {canManage && (
                    <td className="px-4 py-3 text-right">
                      <Button
                        variant="ghost"
                        size="icon-sm"
                        aria-label={`Remove ${member.player_display_name} from team`}
                        onClick={() => setRemoveTarget({ id: member.id, name: member.player_display_name })}
                        className="text-muted-foreground hover:text-destructive"
                      >
                        <Trash2Icon className="size-3.5" />
                      </Button>
                    </td>
                  )}
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      <AddMemberDialog
        open={addOpen}
        onOpenChange={setAddOpen}
        orgSlug={orgSlug}
        teamId={teamId}
        existingMemberPlayerIds={(data?.members ?? []).map((m) => m.player_id)}
      />

      <ConfirmDialog
        open={!!removeTarget}
        onOpenChange={(v) => { if (!v) setRemoveTarget(null) }}
        title="Remove member"
        description={`Remove ${removeTarget?.name ?? "this player"} from the team? Their membership record will be preserved.`}
        confirmLabel="Remove"
        destructive
        isPending={removeMember.isPending}
        onConfirm={handleRemoveConfirm}
      />
    </section>
  )
}

function MembersSkeleton() {
  return (
    <div className="space-y-2" aria-busy="true" aria-label="Loading roster">
      {Array.from({ length: 4 }).map((_, i) => (
        <div key={i} className="flex items-center gap-3 rounded-lg border border-border px-4 py-3">
          <Skeleton className="size-8 rounded-full" />
          <div className="flex-1 space-y-1">
            <Skeleton className="h-4 w-32" />
            <Skeleton className="h-3 w-16" />
          </div>
          <Skeleton className="h-5 w-14 rounded-full" />
        </div>
      ))}
    </div>
  )
}
