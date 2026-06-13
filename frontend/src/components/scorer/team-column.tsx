"use client"

import { useRef, useState } from "react"
import { cn } from "@/lib/utils"
import { RaidPointSheet } from "./raid-point-sheet"
import { AllOutSheet } from "./all-out-sheet"
import {
  buildRaid,
  buildBonus,
  buildTackle,
  buildSuperTackle,
  buildAllOut,
  type AttributedTo,
  type BuiltAction,
} from "@/lib/scoring/scoring-actions"

// Accidental double-fire guard: ignore repeat taps on the same control within
// this window. Prevents rage-tap / sweaty-finger double scoring at the source
// (each intended tap is one client_event_id; the queue dedups resends, but two
// genuine taps would be two events — so we debounce the input itself).
const TAP_DEBOUNCE_MS = 350

interface TeamColumnProps {
  name: string
  opponentName: string
  attribution: AttributedTo
  period: number | null
  disabled: boolean
  onAction: (built: BuiltAction) => void
  align: "left" | "right"
}

export function TeamColumn({
  name,
  opponentName,
  attribution,
  period,
  disabled,
  onAction,
  align,
}: TeamColumnProps) {
  const [raidOpen, setRaidOpen] = useState(false)
  const [allOutOpen, setAllOutOpen] = useState(false)
  const lastTap = useRef<Record<string, number>>({})

  function guarded(key: string, fn: () => void) {
    const now = Date.now()
    if (now - (lastTap.current[key] ?? 0) < TAP_DEBOUNCE_MS) return
    lastTap.current[key] = now
    fn()
  }

  const ctx = { period }

  return (
    <div className={cn("flex flex-col gap-2", align === "right" ? "items-stretch" : "items-stretch")}>
      <ScoreButton
        label="+ Raid"
        sublabel="successful"
        variant="primary"
        disabled={disabled}
        onClick={() => setRaidOpen(true)}
        ariaLabel={`${name} successful raid`}
      />
      <ScoreButton
        label="Bonus +1"
        disabled={disabled}
        onClick={() => guarded("bonus", () => onAction(buildBonus(attribution, ctx)))}
        ariaLabel={`${name} bonus point`}
      />
      <ScoreButton
        label="Tackle +1"
        disabled={disabled}
        onClick={() => guarded("tackle", () => onAction(buildTackle(attribution, ctx)))}
        ariaLabel={`${name} successful tackle`}
      />
      <ScoreButton
        label="Super Tackle +2"
        disabled={disabled}
        onClick={() => guarded("superTackle", () => onAction(buildSuperTackle(attribution, ctx)))}
        ariaLabel={`${name} super tackle`}
      />
      <ScoreButton
        label="All Out"
        variant="warn"
        disabled={disabled}
        onClick={() => setAllOutOpen(true)}
        ariaLabel={`${name} all out`}
      />

      <RaidPointSheet
        open={raidOpen}
        teamName={name}
        onOpenChange={setRaidOpen}
        // Route through the same tap-guard as the single-tap buttons so a
        // double-fire of the sheet selection can't double-score.
        onSelect={(points) => guarded("raid", () => onAction(buildRaid(attribution, points, ctx)))}
      />
      <AllOutSheet
        open={allOutOpen}
        eliminatedName={name}
        opponentName={opponentName}
        onOpenChange={setAllOutOpen}
        onConfirm={(bonus) => guarded("allOut", () => onAction(buildAllOut(attribution, bonus, ctx)))}
      />
    </div>
  )
}

function ScoreButton({
  label,
  sublabel,
  variant = "default",
  disabled,
  onClick,
  ariaLabel,
}: {
  label: string
  sublabel?: string
  variant?: "default" | "primary" | "warn"
  disabled: boolean
  onClick: () => void
  ariaLabel: string
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      disabled={disabled}
      aria-label={ariaLabel}
      className={cn(
        // ≥56px tall touch target, full column width, high contrast.
        "flex min-h-14 flex-col items-center justify-center rounded-xl border-2 px-2 text-center text-base font-semibold transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:pointer-events-none disabled:opacity-40 active:scale-[0.98]",
        variant === "primary" &&
          "border-primary bg-primary text-primary-foreground hover:bg-primary/90",
        variant === "warn" &&
          "border-amber-400 bg-amber-50 text-amber-800 hover:bg-amber-100 dark:bg-amber-950/40 dark:text-amber-200",
        variant === "default" && "border-border bg-card hover:bg-accent",
      )}
    >
      <span>{label}</span>
      {sublabel && <span className="text-xs font-normal opacity-80">{sublabel}</span>}
    </button>
  )
}
