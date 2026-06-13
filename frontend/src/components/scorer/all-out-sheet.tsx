"use client"

import { useState } from "react"
import { MinusIcon, PlusIcon } from "lucide-react"
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from "@/components/ui/dialog"
import { Button } from "@/components/ui/button"

interface AllOutSheetProps {
  open: boolean
  eliminatedName: string
  opponentName: string
  onOpenChange: (open: boolean) => void
  onConfirm: (bonusPoints: number) => void
}

const DEFAULT_BONUS = 2

/**
 * All-out confirmation. The eliminated→opponent attribution is the most
 * counter-intuitive scoring rule, so it is always confirmed and the resulting
 * award is spelled out in plain language to prevent inverted attribution.
 */
export function AllOutSheet({
  open,
  eliminatedName,
  opponentName,
  onOpenChange,
  onConfirm,
}: AllOutSheetProps) {
  const [bonus, setBonus] = useState(DEFAULT_BONUS)

  // Reset to the default on close (in an event handler, not an effect) so the
  // next all-out starts at the standard 2.
  function handleOpenChange(next: boolean) {
    if (!next) setBonus(DEFAULT_BONUS)
    onOpenChange(next)
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="max-w-xs">
        <DialogHeader>
          <DialogTitle>Confirm all out</DialogTitle>
          <DialogDescription>
            <strong>{eliminatedName}</strong> all out →{" "}
            <strong>{opponentName}</strong> score{" "}
            <span className="tabular-nums">+{bonus}</span>
          </DialogDescription>
        </DialogHeader>

        <div className="flex items-center justify-center gap-4 py-3">
          <button
            type="button"
            onClick={() => setBonus((b) => Math.max(1, b - 1))}
            aria-label="Decrease bonus points"
            className="flex size-12 items-center justify-center rounded-full border border-border text-foreground transition-colors hover:bg-accent disabled:opacity-40"
            disabled={bonus <= 1}
          >
            <MinusIcon className="size-5" />
          </button>
          <span className="w-10 text-center text-3xl font-bold tabular-nums" aria-live="polite">
            {bonus}
          </span>
          <button
            type="button"
            onClick={() => setBonus((b) => Math.min(10, b + 1))}
            aria-label="Increase bonus points"
            className="flex size-12 items-center justify-center rounded-full border border-border text-foreground transition-colors hover:bg-accent disabled:opacity-40"
            disabled={bonus >= 10}
          >
            <PlusIcon className="size-5" />
          </button>
        </div>

        <DialogFooter className="gap-2 sm:gap-0">
          <Button variant="outline" onClick={() => handleOpenChange(false)}>
            Cancel
          </Button>
          <Button
            onClick={() => {
              onConfirm(bonus)
              handleOpenChange(false)
            }}
          >
            Confirm all out
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
