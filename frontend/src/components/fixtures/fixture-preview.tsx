"use client"

import type { GenerateResponse, PreviewSlot } from "@/types/api/fixtures"
import type { ParticipantType } from "@/types/api/tournaments"

interface SlotResolver {
  (slot: PreviewSlot): string
}

/**
 * Builds a slot-label resolver: participant slots resolve to a team/player name,
 * qualifier slots read like "Group A #1", and TBD slots read "TBD".
 */
export function makeSlotResolver(
  resolve: (teamId: string | null, playerId: string | null) => string,
  participantType: ParticipantType,
): SlotResolver {
  return (slot: PreviewSlot) => {
    if (slot.kind === "qualifier") return `Group ${slot.group} #${slot.rank}`
    if (slot.kind === "tbd" || !slot.participant_id) return "TBD"
    return participantType === "team"
      ? resolve(slot.participant_id, null)
      : resolve(null, slot.participant_id)
  }
}

interface FixturePreviewProps {
  preview: GenerateResponse
  resolveSlot: SlotResolver
}

/**
 * Read-only rendering of a generation preview: matches grouped by round (and by
 * group label for group stages), so a director sees exactly what will be created.
 */
export function FixturePreview({ preview, resolveSlot }: FixturePreviewProps) {
  // Group matches by a section key: group label (group stage) or round name.
  const sections = new Map<string, typeof preview.matches>()
  for (const m of preview.matches) {
    const key = m.group_label ? `Group ${m.group_label}` : m.round_name
    const arr = sections.get(key) ?? []
    arr.push(m)
    sections.set(key, arr)
  }

  return (
    <div className="space-y-4">
      {[...sections.entries()].map(([section, matches]) => (
        <div key={section}>
          <h4 className="mb-1.5 text-xs font-semibold uppercase tracking-wide text-muted-foreground">
            {section}
          </h4>
          <ul className="divide-y divide-border rounded-md border border-border">
            {matches.map((m) => (
              <li key={m.match_number} className="flex items-center gap-2 px-3 py-2 text-sm">
                <span className="w-8 shrink-0 tabular-nums text-xs text-muted-foreground">
                  #{m.match_number}
                </span>
                <span className="min-w-0 flex-1 truncate">
                  {resolveSlot(m.home)} <span className="text-muted-foreground">vs</span> {resolveSlot(m.away)}
                </span>
                {m.next_match_number != null && (
                  <span className="shrink-0 text-xs text-muted-foreground">→ #{m.next_match_number}</span>
                )}
              </li>
            ))}
          </ul>
        </div>
      ))}

      {preview.byes && preview.byes.length > 0 && (
        <p className="text-xs text-muted-foreground">
          {preview.byes.length} bye{preview.byes.length === 1 ? "" : "s"} (top seeds advance directly).
        </p>
      )}
    </div>
  )
}

/** Counts the distinct rounds in a preview (for the structure summary). */
export function roundCount(preview: GenerateResponse): number {
  const rounds = new Set<number>()
  for (const m of preview.matches) rounds.add(m.round_number)
  return rounds.size
}
