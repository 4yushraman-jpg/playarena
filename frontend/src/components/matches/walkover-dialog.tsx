"use client"

import { useState } from "react"
import { Loader2Icon, FlagIcon } from "lucide-react"
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from "@/components/ui/dialog"
import { Button } from "@/components/ui/button"
import { Label } from "@/components/ui/label"
import { cn } from "@/lib/utils"

export type WalkoverWinner = "home" | "away"

interface WalkoverDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  homeName: string
  awayName: string
  isPending: boolean
  onConfirm: (winner: WalkoverWinner, reason: string) => void
}

/**
 * Awards a walkover: an administrative win when one side does not appear or
 * withdraws. The organizer picks the side that IS present (the winner) and must
 * give a reason. The winner choice + non-empty reason are required before the
 * action enables — mirroring the backend validation so a forfeit is never
 * recorded without an explanation.
 */
export function WalkoverDialog({
  open,
  onOpenChange,
  homeName,
  awayName,
  isPending,
  onConfirm,
}: WalkoverDialogProps) {
  const [winner, setWinner] = useState<WalkoverWinner | null>(null)
  const [reason, setReason] = useState("")

  const trimmedReason = reason.trim()
  const canSubmit = winner !== null && trimmedReason.length > 0 && !isPending

  // Reset transient state whenever the dialog is closed so a reopen starts clean.
  function handleOpenChange(next: boolean) {
    if (!next) {
      setWinner(null)
      setReason("")
    }
    onOpenChange(next)
  }

  function handleConfirm() {
    if (winner === null || trimmedReason.length === 0) return
    onConfirm(winner, trimmedReason)
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="max-w-md">
        <DialogHeader>
          <DialogTitle>Award walkover</DialogTitle>
          <DialogDescription>
            Use this when one side does not appear or withdraws. The winner is
            recorded with a 0–0 forfeit score. This cannot be undone.
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4">
          <div className="space-y-2">
            <Label>Winner (the side present)</Label>
            <div className="grid grid-cols-2 gap-2">
              <WinnerButton
                label={homeName}
                selected={winner === "home"}
                onSelect={() => setWinner("home")}
              />
              <WinnerButton
                label={awayName}
                selected={winner === "away"}
                onSelect={() => setWinner("away")}
              />
            </div>
          </div>

          <div className="space-y-2">
            <Label htmlFor="walkover-reason">Reason</Label>
            <textarea
              id="walkover-reason"
              value={reason}
              onChange={(e) => setReason(e.target.value)}
              rows={3}
              placeholder="e.g. Away team failed to appear; 15-minute grace expired"
              className="flex w-full rounded-md border border-input bg-transparent px-3 py-2 text-sm shadow-sm placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-50"
              disabled={isPending}
            />
          </div>
        </div>

        <DialogFooter className="gap-2 sm:gap-0">
          <Button variant="outline" onClick={() => handleOpenChange(false)} disabled={isPending}>
            Cancel
          </Button>
          <Button onClick={handleConfirm} disabled={!canSubmit} className="gap-2">
            {isPending ? <Loader2Icon className="size-4 animate-spin" /> : <FlagIcon className="size-4" />}
            Award walkover
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

function WinnerButton({
  label,
  selected,
  onSelect,
}: {
  label: string
  selected: boolean
  onSelect: () => void
}) {
  return (
    <button
      type="button"
      onClick={onSelect}
      aria-pressed={selected}
      className={cn(
        "truncate rounded-md border px-3 py-2 text-sm font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring",
        selected
          ? "border-primary bg-primary/10 text-primary"
          : "border-input hover:bg-accent",
      )}
    >
      {label}
    </button>
  )
}
