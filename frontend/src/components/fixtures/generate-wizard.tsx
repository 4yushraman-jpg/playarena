"use client"

import { useState } from "react"
import { Loader2Icon, CheckCircle2Icon } from "lucide-react"
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
import { Input } from "@/components/ui/input"
import { cn } from "@/lib/utils"
import { useParticipantNames } from "@/hooks/use-participant-names"
import { usePreviewFixtures, useGenerateFixtures } from "@/hooks/use-fixture-generation"
import { FixturePreview, makeSlotResolver, roundCount } from "./fixture-preview"
import type { GenerationFormat, SeedStrategy, GenerateResponse } from "@/types/api/fixtures"
import type { Tournament } from "@/types/api/tournaments"

const FORMATS: { value: GenerationFormat; label: string; desc: string }[] = [
  { value: "round_robin", label: "Round Robin", desc: "Everyone plays everyone once." },
  { value: "league", label: "League", desc: "Double round-robin (home & away)." },
  { value: "knockout", label: "Knockout", desc: "Single-elimination bracket with seeding." },
  { value: "group_knockout", label: "Group + Knockout", desc: "Group stage, then a knockout." },
]

const STRATEGIES: { value: SeedStrategy; label: string; desc: string }[] = [
  { value: "registration", label: "Registration order", desc: "Seed by sign-up time." },
  { value: "manual", label: "Manual seeds", desc: "Use organiser-assigned seed numbers." },
  { value: "random", label: "Random", desc: "Deterministic shuffle (seed stored for reproducibility)." },
]

const STEPS = ["Format", "Settings", "Structure", "Preview", "Confirm"]

interface GenerateWizardProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  orgSlug: string
  tournament: Tournament
}

function defaultFormat(t: Tournament): GenerationFormat {
  switch (t.format) {
    case "round_robin":
    case "league":
    case "knockout":
    case "group_knockout":
      return t.format
    default:
      return "round_robin" // double_elimination is not supported in FE-8C
  }
}

export function GenerateWizard({ open, onOpenChange, orgSlug, tournament }: GenerateWizardProps) {
  const [step, setStep] = useState(0)
  const [format, setFormat] = useState<GenerationFormat>(defaultFormat(tournament))
  const [seedStrategy, setSeedStrategy] = useState<SeedStrategy>("registration")
  const [groups, setGroups] = useState(4)
  const [qualifiers, setQualifiers] = useState(2)
  const [preview, setPreview] = useState<GenerateResponse | null>(null)

  const previewMutation = usePreviewFixtures(orgSlug, tournament.id)
  const generateMutation = useGenerateFixtures(orgSlug, tournament.id)
  const { resolve } = useParticipantNames(orgSlug)
  const resolveSlot = makeSlotResolver(resolve, tournament.participant_type)

  function reset() {
    setStep(0)
    setFormat(defaultFormat(tournament))
    setSeedStrategy("registration")
    setGroups(4)
    setQualifiers(2)
    setPreview(null)
  }

  function handleOpenChange(next: boolean) {
    if (!next) reset()
    onOpenChange(next)
  }

  function buildBody() {
    return {
      format,
      seed_strategy: seedStrategy,
      ...(format === "group_knockout"
        ? { groups, qualifiers_per_group: qualifiers }
        : {}),
      ...(preview?.random_seed != null ? { random_seed: preview.random_seed } : {}),
    }
  }

  // Settings → Structure: fetch the preview once, then advance.
  function goToReview() {
    previewMutation.mutate(
      {
        format,
        seed_strategy: seedStrategy,
        ...(format === "group_knockout" ? { groups, qualifiers_per_group: qualifiers } : {}),
      },
      {
        onSuccess: (data) => {
          setPreview(data)
          setStep(2)
        },
      },
    )
  }

  function confirm() {
    generateMutation.mutate(
      {
        ...buildBody(),
        expected_participants: preview?.seed_order.length,
      },
      { onSuccess: () => handleOpenChange(false) },
    )
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="max-h-[85vh] max-w-lg overflow-hidden">
        <DialogHeader>
          <DialogTitle>Generate fixtures</DialogTitle>
          <DialogDescription>
            Step {step + 1} of {STEPS.length}: {STEPS[step]}. Nothing is created until you confirm.
          </DialogDescription>
        </DialogHeader>

        {/* Stepper */}
        <ol className="flex items-center gap-1 text-[11px] text-muted-foreground" aria-hidden="true">
          {STEPS.map((s, i) => (
            <li key={s} className={cn("flex-1 rounded-full py-1 text-center", i <= step ? "bg-primary/15 text-primary" : "bg-muted")}>
              {s}
            </li>
          ))}
        </ol>

        <div className="max-h-[50vh] overflow-y-auto pr-1">
          {step === 0 && (
            <RadioCards
              label="Format"
              options={FORMATS}
              value={format}
              onChange={(v) => setFormat(v as GenerationFormat)}
            />
          )}

          {step === 1 && (
            <div className="space-y-4">
              <RadioCards
                label="Seeding"
                options={STRATEGIES}
                value={seedStrategy}
                onChange={(v) => setSeedStrategy(v as SeedStrategy)}
              />
              {format === "group_knockout" && (
                <div className="grid grid-cols-2 gap-3">
                  <div className="space-y-1.5">
                    <Label htmlFor="wiz-groups">Groups</Label>
                    <Input id="wiz-groups" type="number" min={2} value={groups}
                      onChange={(e) => setGroups(Math.max(2, Number(e.target.value) || 2))} />
                  </div>
                  <div className="space-y-1.5">
                    <Label htmlFor="wiz-qual">Qualify / group</Label>
                    <Input id="wiz-qual" type="number" min={1} value={qualifiers}
                      onChange={(e) => setQualifiers(Math.max(1, Number(e.target.value) || 1))} />
                  </div>
                </div>
              )}
            </div>
          )}

          {step === 2 && preview && <StructureSummary preview={preview} />}

          {step === 3 && preview && <FixturePreview preview={preview} resolveSlot={resolveSlot} />}

          {step === 4 && preview && (
            <div className="space-y-3">
              <div className="flex items-start gap-2 rounded-md border border-border bg-muted/40 p-3 text-sm">
                <CheckCircle2Icon className="mt-0.5 size-4 shrink-0 text-primary" />
                <div>
                  <p className="font-medium">Ready to generate {preview.matches.length} fixtures.</p>
                  <p className="text-xs text-muted-foreground">
                    {formatLabel(format)} · {seedLabel(seedStrategy)}
                    {preview.random_seed != null ? ` · seed ${preview.random_seed}` : ""}
                  </p>
                  <p className="mt-1 text-xs text-muted-foreground">
                    This cannot be undone. Set the tournament to Ongoing afterwards to start play.
                  </p>
                </div>
              </div>
            </div>
          )}
        </div>

        <DialogFooter className="flex-row justify-between gap-2 sm:justify-between">
          <Button
            variant="outline"
            onClick={() => (step === 0 ? handleOpenChange(false) : setStep((s) => s - 1))}
            disabled={previewMutation.isPending || generateMutation.isPending}
          >
            {step === 0 ? "Cancel" : "Back"}
          </Button>

          {step === 1 ? (
            <Button onClick={goToReview} disabled={previewMutation.isPending} className="gap-2">
              {previewMutation.isPending && <Loader2Icon className="size-4 animate-spin" />}
              Preview
            </Button>
          ) : step < 4 ? (
            <Button onClick={() => setStep((s) => s + 1)} disabled={step >= 2 && !preview}>
              Next
            </Button>
          ) : (
            <Button onClick={confirm} disabled={generateMutation.isPending} className="gap-2">
              {generateMutation.isPending && <Loader2Icon className="size-4 animate-spin" />}
              Generate fixtures
            </Button>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

function StructureSummary({ preview }: { preview: GenerateResponse }) {
  const rows: [string, string][] = [
    ["Format", formatLabel(preview.format)],
    ["Seeding", seedLabel(preview.seed_strategy)],
    ["Participants", String(preview.seed_order.length)],
    ["Matches", String(preview.matches.length)],
    ["Rounds", String(roundCount(preview))],
  ]
  if (preview.random_seed != null) rows.push(["Random seed", String(preview.random_seed)])
  if (preview.byes && preview.byes.length) rows.push(["Byes", String(preview.byes.length)])
  return (
    <div className="space-y-3">
      <dl className="grid grid-cols-2 gap-2 text-sm">
        {rows.map(([k, v]) => (
          <div key={k} className="flex flex-col">
            <dt className="text-xs text-muted-foreground">{k}</dt>
            <dd className="font-medium">{v}</dd>
          </div>
        ))}
      </dl>
      {preview.warnings && preview.warnings.length > 0 && (
        <ul className="space-y-1 rounded-md border border-amber-200 bg-amber-50 p-2 text-xs text-amber-800 dark:border-amber-800 dark:bg-amber-950/30 dark:text-amber-300">
          {preview.warnings.map((w, i) => (
            <li key={i}>⚠ {w}</li>
          ))}
        </ul>
      )}
    </div>
  )
}

function RadioCards<T extends string>({
  label,
  options,
  value,
  onChange,
}: {
  label: string
  options: { value: T; label: string; desc: string }[]
  value: T
  onChange: (v: T) => void
}) {
  return (
    <div className="space-y-2">
      <Label>{label}</Label>
      <div className="space-y-2">
        {options.map((o) => (
          <button
            key={o.value}
            type="button"
            onClick={() => onChange(o.value)}
            aria-pressed={value === o.value}
            className={cn(
              "flex w-full flex-col rounded-md border px-3 py-2 text-left transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring",
              value === o.value ? "border-primary bg-primary/5" : "border-input hover:bg-accent",
            )}
          >
            <span className="text-sm font-medium">{o.label}</span>
            <span className="text-xs text-muted-foreground">{o.desc}</span>
          </button>
        ))}
      </div>
    </div>
  )
}

function formatLabel(f: string): string {
  return FORMATS.find((x) => x.value === f)?.label ?? f
}
function seedLabel(s: string): string {
  return STRATEGIES.find((x) => x.value === s)?.label ?? s
}
