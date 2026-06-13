"use client"

import { Loader2Icon, TrophyIcon } from "lucide-react"
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from "@/components/ui/dialog"
import { Button } from "@/components/ui/button"
import type { CompletionWinner } from "@/lib/scoring/completion-gate"

interface CompleteMatchDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  homeName: string
  awayName: string
  homeScore: number
  awayScore: number
  winner: CompletionWinner | null
  isPending: boolean
  onConfirm: () => void
}

/**
 * Deliberate completion confirmation. The winner shown is derived from the
 * AUTHORITATIVE server score (never the optimistic display), so what the
 * organizer confirms is exactly what the backend will record. Completion is
 * only reachable when the gate is satisfied (synced, no failures, score
 * settled), making accidental or partial completion impossible.
 */
export function CompleteMatchDialog({
  open,
  onOpenChange,
  homeName,
  awayName,
  homeScore,
  awayScore,
  winner,
  isPending,
  onConfirm,
}: CompleteMatchDialogProps) {
  const resultLine =
    winner == null || winner.side === "draw"
      ? `Draw — ${homeScore}–${awayScore}`
      : winner.side === "home"
        ? `${homeName} win ${homeScore}–${awayScore}`
        : `${awayName} win ${awayScore}–${homeScore}`

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-sm">
        <DialogHeader>
          <DialogTitle>Complete match?</DialogTitle>
          <DialogDescription>
            This records the final, official result. It cannot be undone here.
          </DialogDescription>
        </DialogHeader>

        <div className="flex flex-col items-center gap-2 rounded-xl border border-border bg-muted/40 py-5">
          <TrophyIcon className="size-6 text-primary" />
          <p className="text-center text-lg font-bold">{resultLine}</p>
          <p className="text-xs text-muted-foreground">Confirmed from the authoritative event log.</p>
        </div>

        <DialogFooter className="gap-2 sm:gap-0">
          <Button variant="outline" onClick={() => onOpenChange(false)} disabled={isPending}>
            Keep scoring
          </Button>
          <Button onClick={onConfirm} disabled={isPending} className="gap-2">
            {isPending && <Loader2Icon className="size-4 animate-spin" />}
            Complete match
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
