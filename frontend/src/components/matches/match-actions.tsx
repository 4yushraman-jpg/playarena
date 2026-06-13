"use client"

import { useState } from "react"
import { useRouter } from "next/navigation"
import Link from "next/link"
import { PencilIcon, XCircleIcon, FlagIcon } from "lucide-react"
import { Button } from "@/components/ui/button"
import { ConfirmDialog } from "@/components/ui/confirm-dialog"
import { WalkoverDialog, type WalkoverWinner } from "./walkover-dialog"
import { useDeleteMatch, useWalkoverMatch } from "@/hooks/use-matches"
import { isFixtureEditable } from "@/lib/match-meta"
import type { Match } from "@/types/api/matches"

interface MatchActionsProps {
  match: Match
  orgSlug: string
  canUpdate: boolean
  canDelete: boolean
  homeName: string
  awayName: string
}

/**
 * Fixture-management actions for the match detail page.
 *
 * - Edit / cancel: scheduled fixtures only (live and terminal matches are owned
 *   by the live-scoring surface).
 * - Award walkover: scheduled OR live matches (a no-show before kickoff, or a
 *   withdrawal mid-match). Terminal matches expose no actions.
 */
export function MatchActions({
  match,
  orgSlug,
  canUpdate,
  canDelete,
  homeName,
  awayName,
}: MatchActionsProps) {
  const router = useRouter()
  const [cancelOpen, setCancelOpen] = useState(false)
  const [walkoverOpen, setWalkoverOpen] = useState(false)
  const deleteMatch = useDeleteMatch(orgSlug)
  const walkoverMatch = useWalkoverMatch(orgSlug, match.id)

  const editable = isFixtureEditable(match)
  const showEdit = canUpdate && editable
  const showCancel = canDelete && editable
  // A walkover can be awarded any time before a match concludes — both before
  // kickoff (scheduled) and after a withdrawal mid-match (live).
  const walkoverable = match.status === "scheduled" || match.status === "live"
  const showWalkover = canUpdate && walkoverable

  if (!showEdit && !showCancel && !showWalkover) return null

  function handleCancel() {
    deleteMatch.mutate(match.id, {
      onSuccess: () => {
        setCancelOpen(false)
        router.refresh()
      },
    })
  }

  function handleWalkover(winner: WalkoverWinner, reason: string) {
    walkoverMatch.mutate(
      { winner, reason },
      {
        onSuccess: () => {
          setWalkoverOpen(false)
          router.refresh()
        },
      },
    )
  }

  return (
    <div className="flex flex-wrap items-center gap-2">
      {showEdit && (
        <Button asChild variant="outline" size="sm" className="gap-1.5">
          <Link href={`/${orgSlug}/matches/${match.id}/edit`}>
            <PencilIcon className="size-3.5" />
            Edit fixture
          </Link>
        </Button>
      )}
      {showWalkover && (
        <Button
          variant="outline"
          size="sm"
          className="gap-1.5"
          onClick={() => setWalkoverOpen(true)}
        >
          <FlagIcon className="size-3.5" />
          Award walkover
        </Button>
      )}
      {showCancel && (
        <Button
          variant="outline"
          size="sm"
          className="gap-1.5 text-destructive hover:text-destructive"
          onClick={() => setCancelOpen(true)}
        >
          <XCircleIcon className="size-3.5" />
          Cancel fixture
        </Button>
      )}

      <ConfirmDialog
        open={cancelOpen}
        onOpenChange={setCancelOpen}
        title="Cancel this fixture?"
        description="The match will be marked as cancelled. This cannot be undone."
        confirmLabel="Cancel fixture"
        cancelLabel="Keep fixture"
        destructive
        isPending={deleteMatch.isPending}
        onConfirm={handleCancel}
      />

      {showWalkover && (
        <WalkoverDialog
          open={walkoverOpen}
          onOpenChange={setWalkoverOpen}
          homeName={homeName}
          awayName={awayName}
          isPending={walkoverMatch.isPending}
          onConfirm={handleWalkover}
        />
      )}
    </div>
  )
}
