"use client"

import { Undo2Icon, Loader2Icon } from "lucide-react"
import { eventLabel } from "./event-labels"
import type { UndoTarget } from "@/lib/scoring/queue-reducer"

interface UndoButtonProps {
  target: UndoTarget
  onUndo: () => void
}

/**
 * Always-visible undo of the most recent still-counting scoring action.
 *  - pending (un-sent) action → removed locally (works offline);
 *  - confirmed action → a score_correction is enqueued;
 *  - in-flight ("busy") → briefly disabled so we never undo the wrong action.
 */
export function UndoButton({ target, onUndo }: UndoButtonProps) {
  const busy = target.kind === "busy"
  const disabled = target.kind === "none" || busy

  const describe =
    target.kind === "remove" || target.kind === "correct"
      ? eventLabel(target.describe.body.event_type)
      : null

  return (
    <button
      type="button"
      onClick={onUndo}
      disabled={disabled}
      aria-label={describe ? `Undo ${describe}` : "Undo"}
      className="flex min-h-12 items-center justify-center gap-2 rounded-xl border-2 border-border bg-card px-4 text-sm font-semibold transition-colors hover:bg-accent focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:pointer-events-none disabled:opacity-40 active:scale-[0.98]"
    >
      {busy ? <Loader2Icon className="size-4 animate-spin" /> : <Undo2Icon className="size-4" />}
      <span>{describe ? `Undo ${describe}` : "Undo"}</span>
    </button>
  )
}
