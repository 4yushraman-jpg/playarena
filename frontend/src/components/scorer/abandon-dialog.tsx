"use client"

import { Loader2Icon, OctagonAlertIcon } from "lucide-react"
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from "@/components/ui/dialog"
import { Button } from "@/components/ui/button"

interface AbandonDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  isPending: boolean
  onConfirm: () => void
}

/**
 * Abandon = the match stopped without a result (weather, crowd, safety). It is
 * terminal and records no winner. Gated like completion (no unsynced events) so
 * the event log is complete up to the point of abandonment.
 */
export function AbandonDialog({ open, onOpenChange, isPending, onConfirm }: AbandonDialogProps) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-sm">
        <DialogHeader>
          <DialogTitle>Abandon match?</DialogTitle>
          <DialogDescription>
            The match will be marked <strong>abandoned</strong> with no winner. This is for matches
            that cannot be finished. It is terminal and cannot be undone here.
          </DialogDescription>
        </DialogHeader>

        <div className="flex items-start gap-2.5 rounded-lg border border-orange-200 bg-orange-50 px-3 py-2.5 text-sm text-orange-800 dark:border-orange-900 dark:bg-orange-950/30 dark:text-orange-200">
          <OctagonAlertIcon className="mt-0.5 size-4 shrink-0" />
          <span>Use this only when the match genuinely cannot be completed.</span>
        </div>

        <DialogFooter className="gap-2 sm:gap-0">
          <Button variant="outline" onClick={() => onOpenChange(false)} disabled={isPending}>
            Keep match
          </Button>
          <Button variant="destructive" onClick={onConfirm} disabled={isPending} className="gap-2">
            {isPending && <Loader2Icon className="size-4 animate-spin" />}
            Abandon match
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
