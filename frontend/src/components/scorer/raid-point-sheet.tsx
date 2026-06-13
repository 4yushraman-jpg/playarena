"use client"

import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
} from "@/components/ui/dialog"

interface RaidPointSheetProps {
  open: boolean
  teamName: string
  onOpenChange: (open: boolean) => void
  onSelect: (points: number) => void
}

const POINT_OPTIONS = [1, 2, 3]

/**
 * Point picker for a successful raid (the one scoring action that needs a
 * value). Big targets, one tap to record. Defaults are 1–3 (covers virtually
 * all Kabaddi raids). Selecting a value immediately enqueues the event.
 */
export function RaidPointSheet({ open, teamName, onOpenChange, onSelect }: RaidPointSheetProps) {
  function choose(points: number) {
    onSelect(points)
    onOpenChange(false)
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-xs">
        <DialogHeader>
          <DialogTitle>Raid points</DialogTitle>
          <DialogDescription>
            Successful raid for <strong>{teamName}</strong> — how many points?
          </DialogDescription>
        </DialogHeader>
        <div className="grid grid-cols-3 gap-2 py-2">
          {POINT_OPTIONS.map((p) => (
            <button
              key={p}
              type="button"
              onClick={() => choose(p)}
              className="flex h-16 items-center justify-center rounded-xl border-2 border-primary/30 bg-primary/5 text-2xl font-bold tabular-nums text-primary transition-colors hover:bg-primary/10 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
              aria-label={`${p} point raid`}
            >
              +{p}
            </button>
          ))}
        </div>
      </DialogContent>
    </Dialog>
  )
}
