"use client"

import { Loader2Icon } from "lucide-react"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Button } from "@/components/ui/button"
import {
  FixtureForm,
  coerceFixtureValues,
  type FixtureFormValues,
} from "./fixture-form"
import { registrationsToParticipants, buildCreateMatchBody } from "./fixture-mapping"
import { useRegistrationList } from "@/hooks/use-registrations"
import { useCreateMatch } from "@/hooks/use-matches"
import type { Tournament } from "@/types/api/tournaments"

interface CreateFixtureDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  tournament: Tournament
  orgSlug: string
}

const PICKER_LIMIT = 100

export function CreateFixtureDialog({
  open,
  onOpenChange,
  tournament,
  orgSlug,
}: CreateFixtureDialogProps) {
  const isTeam = tournament.participant_type === "team"
  const participantNoun = isTeam ? "team" : "player"

  // Only approved registrants are eligible to be fielded in a match; the
  // backend rejects anything else (ErrParticipantNotRegistered).
  const registrationsQuery = useRegistrationList(
    orgSlug,
    tournament.id,
    { status: "approved", limit: PICKER_LIMIT },
    { enabled: open },
  )

  const createMatch = useCreateMatch(orgSlug)

  const participants = registrationsToParticipants(
    registrationsQuery.data?.registrations ?? [],
    isTeam,
  )

  function handleSubmit(values: FixtureFormValues) {
    const body = buildCreateMatchBody(tournament.id, isTeam, coerceFixtureValues(values))
    createMatch.mutate(body, {
      onSuccess: () => onOpenChange(false),
    })
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-lg">
        <DialogHeader>
          <DialogTitle>Create fixture</DialogTitle>
          <DialogDescription>
            Schedule a match between two approved {participantNoun}s in{" "}
            <strong>{tournament.name}</strong>.
          </DialogDescription>
        </DialogHeader>

        {registrationsQuery.isLoading ? (
          <div className="flex items-center gap-2 py-8 text-sm text-muted-foreground">
            <Loader2Icon className="size-4 animate-spin" />
            Loading approved {participantNoun}s…
          </div>
        ) : registrationsQuery.isError ? (
          <div className="flex flex-col items-start gap-2 py-6">
            <p className="text-sm text-destructive">
              Failed to load approved {participantNoun}s.
            </p>
            <Button variant="outline" size="sm" onClick={() => registrationsQuery.refetch()}>
              Retry
            </Button>
          </div>
        ) : (
          <FixtureForm
            participants={participants}
            participantNoun={participantNoun}
            isPending={createMatch.isPending}
            onSubmit={handleSubmit}
            onCancel={() => onOpenChange(false)}
          />
        )}
      </DialogContent>
    </Dialog>
  )
}
