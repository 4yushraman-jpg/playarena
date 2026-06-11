"use client"

import { useId, useState } from "react"
import {
  AlertCircleIcon,
  CheckCircleIcon,
  CheckIcon,
  Loader2Icon,
  SearchIcon,
} from "lucide-react"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { useTeamList } from "@/hooks/use-teams"
import { usePlayerList } from "@/hooks/use-players"
import { useRegisterParticipant, useRegistrationList } from "@/hooks/use-registrations"
import { useDebouncedValue } from "@/hooks/use-debounce"
import { getCapacityUsage, getRegistrationCounts } from "@/lib/registration-stats"
import { cn } from "@/lib/utils"
import type { Tournament } from "@/types/api/tournaments"
import type { TournamentRegistration } from "@/types/api/tournament-registrations"

// ── Eligibility analysis ──────────────────────────────────────────────────────

export interface EligibilityResult {
  canRegister: boolean
  reason?: string
}

/**
 * Mirrors the backend's registration rules (status, window, capacity,
 * duplicates) so users get feedback before submitting. The backend remains
 * the authority — a failed submit still surfaces the server's message.
 *
 * Capacity uses the ACTIVE count (pending + approved), matching the backend's
 * CountActiveRegistrations check. Duplicates block in ANY status (Rule 4):
 * even rejected or withdrawn participants cannot re-register.
 */
export function analyzeEligibility(
  tournament: Tournament,
  existingRegistration: TournamentRegistration | null | undefined,
  now: Date = new Date(),
): EligibilityResult {
  if (tournament.status !== "registration_open") {
    const statusMessages: Record<string, string> = {
      draft: "This tournament hasn't been published yet.",
      registration_closed: "Registrations are closed for this tournament.",
      ongoing: "This tournament is already in progress.",
      completed: "This tournament has ended.",
      cancelled: "This tournament has been cancelled.",
    }
    return {
      canRegister: false,
      reason: statusMessages[tournament.status] ?? "Registrations are not available.",
    }
  }

  if (tournament.registration_opens_at) {
    const opensAt = new Date(tournament.registration_opens_at)
    if (now < opensAt) {
      return {
        canRegister: false,
        reason: `Registrations open on ${opensAt.toLocaleDateString("en-IN", {
          day: "numeric",
          month: "short",
          year: "numeric",
          hour: "2-digit",
          minute: "2-digit",
        })}.`,
      }
    }
  }
  if (tournament.registration_closes_at) {
    const closesAt = new Date(tournament.registration_closes_at)
    if (now > closesAt) {
      return { canRegister: false, reason: "The registration window has closed." }
    }
  }

  const usage = getCapacityUsage(tournament)
  if (usage?.isFull) {
    return {
      canRegister: false,
      reason: `Tournament is full (${usage.used}/${usage.max} active registrations).`,
    }
  }

  if (existingRegistration) {
    const status = existingRegistration.status
    const suffix =
      status === "pending" || status === "approved"
        ? `(registration is ${status})`
        : `(a ${status} registration already exists and cannot be replaced)`
    return {
      canRegister: false,
      reason: `This participant is already registered for this tournament ${suffix}.`,
    }
  }

  return { canRegister: true }
}

/**
 * Hint shown under the picker when the server holds more matches than one
 * page can show. Exported for tests.
 */
export function getPickerOverflowHint(total: number, shown: number): string | null {
  if (total <= shown) return null
  return `Showing first ${shown} of ${total} — type to narrow the list.`
}

// ── Component ─────────────────────────────────────────────────────────────────

const PICKER_PAGE_SIZE = 20

interface RegisterParticipantDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  tournament: Tournament
  orgSlug: string
}

interface PickerItem {
  id: string
  name: string
}

export function RegisterParticipantDialog({
  open,
  onOpenChange,
  tournament,
  orgSlug,
}: RegisterParticipantDialogProps) {
  const isTeam = tournament.participant_type === "team"
  const [search, setSearch] = useState("")
  const debouncedSearch = useDebouncedValue(search, 300)
  const [selected, setSelected] = useState<PickerItem | null>(null)
  const listboxId = useId()

  const teamQuery = useTeamList(
    orgSlug,
    { limit: PICKER_PAGE_SIZE, status: "active", search: debouncedSearch || undefined },
  )
  const playerQuery = usePlayerList(
    orgSlug,
    { limit: PICKER_PAGE_SIZE, status: "active", search: debouncedSearch || undefined },
  )

  const items: PickerItem[] = isTeam
    ? (teamQuery.data?.teams ?? []).map((t) => ({ id: t.id, name: t.name }))
    : (playerQuery.data?.players ?? []).map((p) => ({ id: p.id, name: p.display_name }))
  const pickerTotal = (isTeam ? teamQuery.data?.total : playerQuery.data?.total) ?? items.length
  const pickerLoading = isTeam ? teamQuery.isLoading : playerQuery.isLoading
  const pickerError = isTeam ? teamQuery.isError : playerQuery.isError
  const pickerRetry = isTeam ? teamQuery.refetch : playerQuery.refetch

  // Authoritative duplicate check for the selected participant: ask the server
  // for that participant's registration instead of scanning a truncated page.
  const duplicateQuery = useRegistrationList(
    orgSlug,
    tournament.id,
    isTeam ? { team_id: selected?.id, limit: 1 } : { player_id: selected?.id, limit: 1 },
    { enabled: !!selected },
  )
  const existingRegistration = selected
    ? duplicateQuery.data?.registrations[0] ?? null
    : null
  const duplicateCheckPending = !!selected && duplicateQuery.isLoading

  const registerParticipant = useRegisterParticipant(orgSlug, tournament.id)

  const tournamentEligibility = analyzeEligibility(tournament, null)
  const eligibility = analyzeEligibility(tournament, existingRegistration)

  const counts = getRegistrationCounts(tournament)
  const usage = getCapacityUsage(tournament)
  const overflowHint = getPickerOverflowHint(pickerTotal, items.length)

  const participantNoun = isTeam ? "team" : "player"

  function reset() {
    setSearch("")
    setSelected(null)
  }

  function handleOpenChange(next: boolean) {
    if (!next) reset()
    onOpenChange(next)
  }

  function handleRegister() {
    if (!selected || !eligibility.canRegister || duplicateCheckPending) return
    registerParticipant.mutate(
      isTeam ? { team_id: selected.id } : { player_id: selected.id },
      {
        onSuccess: () => {
          reset()
          onOpenChange(false)
        },
      },
    )
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="max-w-md">
        <DialogHeader>
          <DialogTitle>Register {participantNoun}</DialogTitle>
          <DialogDescription>
            Register a {participantNoun} for <strong>{tournament.name}</strong>.
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4 py-2">
          {/* Tournament-level eligibility banner */}
          {!tournamentEligibility.canRegister && (
            <div
              className="flex items-start gap-2.5 rounded-lg border border-destructive/20 bg-destructive/5 px-3 py-2.5 text-sm text-destructive"
              role="alert"
            >
              <AlertCircleIcon className="mt-0.5 size-4 shrink-0" />
              <span>{tournamentEligibility.reason}</span>
            </div>
          )}

          {/* Capacity info — active = pending + approved, the enforced count */}
          {usage && (
            <div className="flex items-center justify-between rounded-lg bg-muted/50 px-3 py-2 text-xs">
              <span className="text-muted-foreground">Capacity (pending + approved)</span>
              <span className="font-medium tabular-nums">
                {usage.used} / {usage.max}
              </span>
            </div>
          )}
          {!usage && counts.active > 0 && (
            <div className="flex items-center justify-between rounded-lg bg-muted/50 px-3 py-2 text-xs">
              <span className="text-muted-foreground">Active registrations</span>
              <span className="font-medium tabular-nums">{counts.active}</span>
            </div>
          )}

          {/* Participant picker */}
          <div className="space-y-1.5">
            <Label htmlFor={`${listboxId}-search`}>
              {isTeam ? "Select team" : "Select player"}
            </Label>
            <div className="relative">
              <SearchIcon className="pointer-events-none absolute left-2.5 top-1/2 size-3.5 -translate-y-1/2 text-muted-foreground" />
              <Input
                id={`${listboxId}-search`}
                role="combobox"
                aria-expanded="true"
                aria-controls={listboxId}
                placeholder={`Search ${participantNoun}s…`}
                value={search}
                onChange={(e) => setSearch(e.target.value)}
                className="pl-8"
                disabled={!tournamentEligibility.canRegister}
              />
            </div>

            <div
              id={listboxId}
              role="listbox"
              aria-label={`Available ${participantNoun}s`}
              className="max-h-44 overflow-y-auto rounded-lg border border-border"
            >
              {pickerLoading ? (
                <div className="flex items-center gap-2 px-3 py-3 text-sm text-muted-foreground">
                  <Loader2Icon className="size-3.5 animate-spin" />
                  Loading {participantNoun}s…
                </div>
              ) : pickerError ? (
                <div className="flex flex-col items-start gap-2 px-3 py-3">
                  <p className="text-sm text-destructive">
                    Failed to load {participantNoun}s.
                  </p>
                  <Button variant="outline" size="sm" onClick={() => pickerRetry()}>
                    Retry
                  </Button>
                </div>
              ) : items.length === 0 ? (
                <p className="px-3 py-3 text-sm text-muted-foreground">
                  {debouncedSearch
                    ? `No active ${participantNoun}s match “${debouncedSearch}”.`
                    : `No active ${participantNoun}s found in your organization.`}
                </p>
              ) : (
                items.map((item) => {
                  const isSelected = selected?.id === item.id
                  return (
                    <button
                      key={item.id}
                      type="button"
                      role="option"
                      aria-selected={isSelected}
                      className={cn(
                        "flex w-full items-center justify-between px-3 py-2 text-left text-sm transition-colors",
                        isSelected
                          ? "bg-primary/10 font-medium text-primary"
                          : "hover:bg-accent",
                        !tournamentEligibility.canRegister && "pointer-events-none opacity-50",
                      )}
                      onClick={() => setSelected(isSelected ? null : item)}
                    >
                      <span className="truncate">{item.name}</span>
                      {isSelected && <CheckIcon className="size-3.5 shrink-0" />}
                    </button>
                  )
                })
              )}
            </div>
            {overflowHint && (
              <p className="text-xs text-muted-foreground">{overflowHint}</p>
            )}
          </div>

          {/* Per-selection eligibility feedback (aria-live so screen readers
              hear the verdict as soon as the server check resolves) */}
          <div aria-live="polite">
            {selected && duplicateCheckPending && (
              <div className="flex items-center gap-2.5 rounded-lg bg-muted/50 px-3 py-2.5 text-sm text-muted-foreground">
                <Loader2Icon className="size-4 shrink-0 animate-spin" />
                <span>Checking {selected.name}…</span>
              </div>
            )}
            {selected &&
              !duplicateCheckPending &&
              !eligibility.canRegister &&
              tournamentEligibility.canRegister && (
                <div className="flex items-start gap-2.5 rounded-lg border border-destructive/20 bg-destructive/5 px-3 py-2.5 text-sm text-destructive">
                  <AlertCircleIcon className="mt-0.5 size-4 shrink-0" />
                  <span>{eligibility.reason}</span>
                </div>
              )}
            {selected && !duplicateCheckPending && eligibility.canRegister && (
              <div className="flex items-center gap-2.5 rounded-lg border border-green-200 bg-green-50 px-3 py-2.5 text-sm text-green-700 dark:border-green-800 dark:bg-green-950/40 dark:text-green-300">
                <CheckCircleIcon className="size-4 shrink-0" />
                <span>{selected.name} is eligible to register.</span>
              </div>
            )}
          </div>
        </div>

        <DialogFooter className="gap-2 sm:gap-0">
          <Button
            variant="outline"
            onClick={() => handleOpenChange(false)}
            disabled={registerParticipant.isPending}
          >
            Cancel
          </Button>
          <Button
            onClick={handleRegister}
            disabled={
              !selected ||
              !eligibility.canRegister ||
              duplicateCheckPending ||
              registerParticipant.isPending
            }
            className="gap-2"
          >
            {registerParticipant.isPending && (
              <Loader2Icon className="size-3.5 animate-spin" />
            )}
            Register {participantNoun}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
