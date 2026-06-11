"use client"

import { useState } from "react"
import { useRouter } from "next/navigation"
import { toast } from "sonner"
import {
  SendIcon,
  LockIcon,
  PlayIcon,
  CheckCircleIcon,
  XCircleIcon,
  PencilIcon,
  UsersIcon,
} from "lucide-react"
import { Button } from "@/components/ui/button"
import { ConfirmDialog } from "@/components/ui/confirm-dialog"
import { useUpdateTournament, useDeleteTournament } from "@/hooks/use-tournaments"
import type { Tournament, TournamentStatus } from "@/types/api/tournaments"

// ── Transition map ─────────────────────────────────────────────────────────────
// Maps current status → array of available transition actions

interface TransitionAction {
  label: string
  nextStatus: TournamentStatus
  icon: React.ElementType
  variant: "default" | "outline" | "destructive"
  /** Toast copy announced after the transition succeeds. */
  successMessage: string
  confirm?: {
    title: string
    description: string
    confirmLabel: string
    destructive?: boolean
  }
}

function getTransitions(status: TournamentStatus): TransitionAction[] {
  switch (status) {
    case "draft":
      return [
        {
          label: "Publish",
          nextStatus: "registration_open",
          icon: SendIcon,
          variant: "default",
          successMessage: "Tournament published — registrations are open",
          confirm: {
            title: "Publish tournament?",
            description:
              "This will open registrations. Participants will be able to register once published.",
            confirmLabel: "Publish",
          },
        },
      ]
    case "registration_open":
      return [
        {
          label: "Close registrations",
          nextStatus: "registration_closed",
          icon: LockIcon,
          variant: "outline",
          successMessage: "Registrations closed",
          confirm: {
            title: "Close registrations?",
            description:
              "No new teams will be able to register. You can still approve or reject pending registrations.",
            confirmLabel: "Close registrations",
          },
        },
      ]
    case "registration_closed":
      return [
        {
          label: "Start tournament",
          nextStatus: "ongoing",
          icon: PlayIcon,
          variant: "default",
          successMessage: "Tournament started",
          confirm: {
            title: "Start tournament?",
            description:
              "This will mark the tournament as in progress. Make sure all registrations are finalized.",
            confirmLabel: "Start",
          },
        },
      ]
    case "ongoing":
      return [
        {
          label: "Complete tournament",
          nextStatus: "completed",
          icon: CheckCircleIcon,
          variant: "default",
          successMessage: "Tournament completed",
          confirm: {
            title: "Complete tournament?",
            description:
              "This will mark the tournament as completed. This action cannot be undone.",
            confirmLabel: "Complete",
          },
        },
      ]
    default:
      return []
  }
}

// ── Component ─────────────────────────────────────────────────────────────────

interface TournamentActionsProps {
  tournament: Tournament
  orgSlug: string
  canUpdate: boolean
  canDelete: boolean
}

export function TournamentActions({
  tournament,
  orgSlug,
  canUpdate,
  canDelete,
}: TournamentActionsProps) {
  const router = useRouter()
  const updateTournament = useUpdateTournament(orgSlug, tournament.id)
  const deleteTournament = useDeleteTournament(orgSlug)

  const [pendingTransition, setPendingTransition] = useState<TransitionAction | null>(null)
  const [cancelOpen, setCancelOpen] = useState(false)

  const transitions = getTransitions(tournament.status)
  const isCancellable =
    tournament.status !== "cancelled" && tournament.status !== "completed"

  function handleTransition(action: TransitionAction) {
    if (action.confirm) {
      setPendingTransition(action)
    } else {
      updateTournament.mutate(
        { status: action.nextStatus },
        { onSuccess: () => toast.success(action.successMessage) },
      )
    }
  }

  function confirmTransition() {
    if (!pendingTransition) return
    const action = pendingTransition
    updateTournament.mutate(
      { status: action.nextStatus },
      {
        onSuccess: () => {
          setPendingTransition(null)
          toast.success(action.successMessage)
        },
      },
    )
  }

  function confirmCancel() {
    deleteTournament.mutate(tournament.id, {
      onSuccess: () => {
        setCancelOpen(false)
        router.push(`/${orgSlug}/tournaments`)
      },
    })
  }

  if (!canUpdate && !canDelete) return null

  return (
    <>
      <div className="flex flex-wrap items-center gap-2">
        {/* Edit */}
        {canUpdate && tournament.status !== "cancelled" && tournament.status !== "completed" && (
          <Button
            variant="outline"
            size="sm"
            className="gap-1.5"
            onClick={() => router.push(`/${orgSlug}/tournaments/${tournament.id}/edit`)}
          >
            <PencilIcon className="size-3.5" />
            Edit
          </Button>
        )}

        {/* Registrations */}
        {canUpdate && (
          <Button
            variant="outline"
            size="sm"
            className="gap-1.5"
            onClick={() =>
              router.push(`/${orgSlug}/tournaments/${tournament.id}/registrations`)
            }
          >
            <UsersIcon className="size-3.5" />
            Registrations
          </Button>
        )}

        {/* Status transitions */}
        {canUpdate &&
          transitions.map((action) => (
            <Button
              key={action.nextStatus}
              variant={action.variant}
              size="sm"
              className="gap-1.5"
              disabled={updateTournament.isPending}
              onClick={() => handleTransition(action)}
            >
              <action.icon className="size-3.5" />
              {action.label}
            </Button>
          ))}

        {/* Cancel (destructive) */}
        {canDelete && isCancellable && (
          <Button
            variant="destructive"
            size="sm"
            className="gap-1.5"
            onClick={() => setCancelOpen(true)}
          >
            <XCircleIcon className="size-3.5" />
            Cancel tournament
          </Button>
        )}
      </div>

      {/* Transition confirmation dialog */}
      {pendingTransition?.confirm && (
        <ConfirmDialog
          open={!!pendingTransition}
          onOpenChange={(open) => !open && setPendingTransition(null)}
          title={pendingTransition.confirm.title}
          description={pendingTransition.confirm.description}
          confirmLabel={pendingTransition.confirm.confirmLabel}
          destructive={pendingTransition.confirm.destructive}
          isPending={updateTournament.isPending}
          onConfirm={confirmTransition}
        />
      )}

      {/* Cancel confirmation dialog */}
      <ConfirmDialog
        open={cancelOpen}
        onOpenChange={setCancelOpen}
        title="Cancel tournament?"
        description="This will permanently cancel the tournament. All pending registrations will remain, but no new ones can be accepted. This cannot be undone."
        confirmLabel="Cancel tournament"
        destructive
        isPending={deleteTournament.isPending}
        onConfirm={confirmCancel}
      />
    </>
  )
}
