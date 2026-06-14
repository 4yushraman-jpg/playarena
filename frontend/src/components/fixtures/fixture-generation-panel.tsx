"use client"

import { useState } from "react"
import { Wand2Icon, InfoIcon, ArrowRightCircleIcon } from "lucide-react"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { GenerateWizard } from "./generate-wizard"
import { useMatchList } from "@/hooks/use-matches"
import { useGenerationInfo, useResolveQualifiers } from "@/hooks/use-fixture-generation"
import { formatDateTime } from "@/lib/format"
import type { Tournament } from "@/types/api/tournaments"
import type { Match } from "@/types/api/matches"

const FIXTURE_LIMIT = 200

interface FixtureGenerationPanelProps {
  orgSlug: string
  tournament: Tournament
  canManage: boolean
}

/**
 * Tournament-director surface for automated fixtures: generate (when none exist
 * and registration is closed), the explainability/audit record once generated,
 * and qualifier resolution for a completed group stage.
 */
export function FixtureGenerationPanel({ orgSlug, tournament, canManage }: FixtureGenerationPanelProps) {
  const [wizardOpen, setWizardOpen] = useState(false)
  const { data } = useMatchList(orgSlug, { tournament_id: tournament.id, limit: FIXTURE_LIMIT })
  const matches = data?.matches ?? []
  const hasFixtures = matches.length > 0

  const canGenerate =
    canManage && tournament.status === "registration_closed" && !hasFixtures
  const generatedInfo = useGenerationInfo(orgSlug, tournament.id, hasFixtures)

  const resolve = useResolveQualifiers(orgSlug, tournament.id)
  const showResolve =
    canManage &&
    tournament.status === "ongoing" &&
    tournament.format === "group_knockout" &&
    groupStageComplete(matches) &&
    knockoutNeedsResolution(matches)

  // Nothing to show: no generation entry, no audit record, no resolve action.
  if (!canGenerate && !generatedInfo.data?.generated && !showResolve) return null

  return (
    <Card>
      <CardHeader className="pb-3">
        <CardTitle className="flex items-center gap-2 text-base">
          <Wand2Icon className="size-4 text-muted-foreground" />
          Fixture generation
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        {canGenerate && (
          <div className="flex flex-col items-start gap-2">
            <p className="text-sm text-muted-foreground">
              Registration is closed. Generate the full schedule automatically — no
              manual fixture entry required.
            </p>
            <Button className="gap-1.5" onClick={() => setWizardOpen(true)}>
              <Wand2Icon className="size-4" />
              Generate fixtures
            </Button>
          </div>
        )}

        {showResolve && (
          <div className="flex flex-col items-start gap-2">
            <p className="text-sm text-muted-foreground">
              The group stage is complete. Resolve qualifiers to fill the knockout bracket.
            </p>
            <Button
              variant="outline"
              className="gap-1.5"
              onClick={() => resolve.mutate()}
              disabled={resolve.isPending}
            >
              <ArrowRightCircleIcon className="size-4" />
              Resolve qualifiers
            </Button>
          </div>
        )}

        {generatedInfo.data?.generated && (
          <GenerationAudit info={generatedInfo.data} />
        )}
      </CardContent>

      {canGenerate && (
        <GenerateWizard
          open={wizardOpen}
          onOpenChange={setWizardOpen}
          orgSlug={orgSlug}
          tournament={tournament}
        />
      )}
    </Card>
  )
}

function GenerationAudit({ info }: { info: NonNullable<ReturnType<typeof useGenerationInfo>["data"]> }) {
  const rows: [string, string][] = []
  if (info.format) rows.push(["Format", info.format.replace(/_/g, " ")])
  if (info.seed_strategy) rows.push(["Seed strategy", info.seed_strategy])
  if (info.random_seed != null) rows.push(["Random seed", String(info.random_seed)])
  if (info.match_count) rows.push(["Matches", String(info.match_count)])
  if (info.generated_at) rows.push(["Generated", formatDateTime(info.generated_at)])
  return (
    <div className="rounded-md border border-border bg-muted/30 p-3">
      <p className="mb-2 flex items-center gap-1.5 text-xs font-medium text-muted-foreground">
        <InfoIcon className="size-3.5" /> Generation record (reproducible)
      </p>
      <dl className="grid grid-cols-2 gap-2 text-sm">
        {rows.map(([k, v]) => (
          <div key={k} className="flex flex-col">
            <dt className="text-xs text-muted-foreground">{k}</dt>
            <dd className="font-medium capitalize">{v}</dd>
          </div>
        ))}
      </dl>
    </div>
  )
}

const TERMINAL = new Set(["completed", "walkover", "cancelled"])

function groupStageComplete(matches: Match[]): boolean {
  const group = matches.filter((m) => m.group_label)
  return group.length > 0 && group.every((m) => TERMINAL.has(m.status))
}

// knockoutNeedsResolution: the first knockout round still has unfilled slots.
function knockoutNeedsResolution(matches: Match[]): boolean {
  const ko = matches.filter((m) => !m.group_label)
  if (ko.length === 0) return false
  const minRound = Math.min(...ko.map((m) => m.round_number ?? Number.MAX_SAFE_INTEGER))
  return ko.some(
    (m) =>
      (m.round_number ?? -1) === minRound &&
      !m.home_team_id &&
      !m.away_team_id &&
      !m.home_player_id &&
      !m.away_player_id,
  )
}
