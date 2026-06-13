"use client"

import { useState } from "react"
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
} from "@/components/ui/dialog"
import { cn } from "@/lib/utils"
import { buildGeneric, buildPenalty, type BuiltAction } from "@/lib/scoring/scoring-actions"
import type { MatchEventType } from "@/types/api/match-events"

interface MoreEventsSheetProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  isTeam: boolean
  homeName: string
  awayName: string
  homeId: string | null
  awayId: string | null
  period: number | null
  clockSeconds: number | null
  onEnqueue: (built: BuiltAction) => void
}

const PLAYER_EVENTS: { type: MatchEventType; label: string }[] = [
  { type: "player_out", label: "Player out" },
  { type: "player_revived", label: "Player revived" },
  { type: "player_substituted", label: "Substitution" },
  { type: "player_injured", label: "Player injured" },
]

/**
 * Lower-frequency, append-only events kept off the fast scoring surface:
 * timeouts (regular/technical/injury), penalties, and player-state events.
 * Each action emits one event and closes the sheet — keeping the primary loop
 * fast and making accidental repeats unlikely. All carry a client_event_id.
 */
export function MoreEventsSheet({
  open,
  onOpenChange,
  isTeam,
  homeName,
  awayName,
  homeId,
  awayId,
  period,
  clockSeconds,
  onEnqueue,
}: MoreEventsSheetProps) {
  const [penaltyPoints, setPenaltyPoints] = useState(1)

  function emit(built: BuiltAction) {
    onEnqueue(built)
    onOpenChange(false)
  }

  function timeout(kind: string) {
    emit(buildGeneric("timeout_called", { payload: { kind }, period, clockSeconds }))
  }

  function penalty(participantId: string | null) {
    if (!participantId) return
    emit(buildPenalty({ teamMode: isTeam, participantId }, penaltyPoints, { period }))
  }

  function playerEvent(type: MatchEventType) {
    emit(buildGeneric(type, { period, clockSeconds }))
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-sm">
        <DialogHeader>
          <DialogTitle>More events</DialogTitle>
          <DialogDescription>Timeouts, penalties and player events.</DialogDescription>
        </DialogHeader>

        <div className="space-y-5 py-1">
          <Section title="Timeout">
            <div className="grid grid-cols-3 gap-2">
              <SheetButton onClick={() => timeout("regular")}>Timeout</SheetButton>
              <SheetButton onClick={() => timeout("technical")}>Technical</SheetButton>
              <SheetButton onClick={() => timeout("injury")}>Injury</SheetButton>
            </div>
          </Section>

          <Section title="Penalty">
            <div className="mb-2 flex items-center gap-2">
              <span className="text-xs text-muted-foreground">Points</span>
              {[1, 2].map((p) => (
                <button
                  key={p}
                  type="button"
                  onClick={() => setPenaltyPoints(p)}
                  aria-pressed={penaltyPoints === p}
                  className={cn(
                    "flex size-9 items-center justify-center rounded-lg border-2 text-sm font-bold tabular-nums",
                    penaltyPoints === p
                      ? "border-primary bg-primary/10 text-primary"
                      : "border-border",
                  )}
                >
                  {p}
                </button>
              ))}
            </div>
            <div className="grid grid-cols-2 gap-2">
              <SheetButton onClick={() => penalty(homeId)} disabled={!homeId}>
                Award {homeName}
              </SheetButton>
              <SheetButton onClick={() => penalty(awayId)} disabled={!awayId}>
                Award {awayName}
              </SheetButton>
            </div>
          </Section>

          <Section title="Player">
            <div className="grid grid-cols-2 gap-2">
              {PLAYER_EVENTS.map((e) => (
                <SheetButton key={e.type} onClick={() => playerEvent(e.type)}>
                  {e.label}
                </SheetButton>
              ))}
            </div>
          </Section>
        </div>
      </DialogContent>
    </Dialog>
  )
}

function Section({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <div className="space-y-2">
      <h3 className="text-xs font-medium uppercase tracking-wide text-muted-foreground">{title}</h3>
      {children}
    </div>
  )
}

function SheetButton({
  onClick,
  disabled,
  children,
}: {
  onClick: () => void
  disabled?: boolean
  children: React.ReactNode
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      disabled={disabled}
      className="flex min-h-12 items-center justify-center truncate rounded-lg border-2 border-border bg-card px-2 text-sm font-medium transition-colors hover:bg-accent focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:pointer-events-none disabled:opacity-40"
    >
      {children}
    </button>
  )
}
