"use client"

import { useState } from "react"
import { useRouter } from "next/navigation"
import Link from "next/link"
import { PencilIcon, XCircleIcon } from "lucide-react"
import { Button } from "@/components/ui/button"
import { ConfirmDialog } from "@/components/ui/confirm-dialog"
import { useDeleteMatch } from "@/hooks/use-matches"
import { isFixtureEditable } from "@/lib/match-meta"
import type { Match } from "@/types/api/matches"

interface MatchActionsProps {
  match: Match
  orgSlug: string
  canUpdate: boolean
  canDelete: boolean
}

/**
 * Fixture-management actions for the match detail page. Only scheduled fixtures
 * can be edited or cancelled here — live and terminal matches are owned by the
 * live-scoring surface (FE-7B), so no actions render for them.
 */
export function MatchActions({ match, orgSlug, canUpdate, canDelete }: MatchActionsProps) {
  const router = useRouter()
  const [cancelOpen, setCancelOpen] = useState(false)
  const deleteMatch = useDeleteMatch(orgSlug)

  const editable = isFixtureEditable(match)
  const showEdit = canUpdate && editable
  const showCancel = canDelete && editable

  if (!showEdit && !showCancel) return null

  function handleCancel() {
    deleteMatch.mutate(match.id, {
      onSuccess: () => {
        setCancelOpen(false)
        router.refresh()
      },
    })
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
    </div>
  )
}
