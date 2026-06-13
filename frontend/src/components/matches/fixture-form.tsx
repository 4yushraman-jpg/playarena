"use client"

import { useEffect } from "react"
import { useForm, useWatch } from "react-hook-form"
import { zodResolver } from "@hookform/resolvers/zod"
import { z } from "zod"
import { Loader2Icon } from "lucide-react"
import { Button } from "@/components/ui/button"
import {
  FormField,
  FormSelect,
  FormDatePicker,
  type SelectOption,
} from "@/components/ui/form-field"
import { Separator } from "@/components/ui/separator"

// ── Participant option ─────────────────────────────────────────────────────────

export interface FixtureParticipant {
  id: string
  name: string
}

// ── Schema ─────────────────────────────────────────────────────────────────────

const INT_RE = /^\d+$/

const schema = z
  .object({
    home_participant_id: z.string().min(1, "Select the home participant"),
    away_participant_id: z.string().min(1, "Select the away participant"),
    scheduled_at: z.string().min(1, "A scheduled date and time is required"),
    venue: z.string().max(200, "Venue is too long").optional().or(z.literal("")),
    round_name: z.string().max(100, "Round name is too long").optional().or(z.literal("")),
    round_number: z
      .string()
      .refine((v) => v === "" || (INT_RE.test(v) && Number(v) >= 1 && Number(v) <= 1000), {
        message: "Must be a whole number between 1 and 1000",
      })
      .optional()
      .or(z.literal("")),
    match_number: z
      .string()
      .refine((v) => v === "" || (INT_RE.test(v) && Number(v) >= 1 && Number(v) <= 10000), {
        message: "Must be a whole number between 1 and 10000",
      })
      .optional()
      .or(z.literal("")),
  })
  .refine((d) => d.home_participant_id !== d.away_participant_id, {
    message: "Home and away must be different participants",
    path: ["away_participant_id"],
  })

export type FixtureFormValues = z.infer<typeof schema>

// ── Coercion ───────────────────────────────────────────────────────────────────

export interface CoercedFixtureValues {
  homeId: string
  awayId: string
  scheduledAt: string // RFC3339 UTC
  venue?: string
  roundName?: string
  roundNumber?: number
  matchNumber?: number
}

// Converts datetime-local "YYYY-MM-DDTHH:mm" → RFC3339 UTC.
function localToRFC3339(local: string): string | undefined {
  if (!local) return undefined
  const d = new Date(local)
  if (Number.isNaN(d.getTime())) return undefined
  return d.toISOString()
}

// Converts RFC3339 UTC → "YYYY-MM-DDTHH:mm" for the datetime-local input.
export function rfc3339ToLocal(iso: string | null | undefined): string {
  if (!iso) return ""
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return ""
  const pad = (n: number) => String(n).padStart(2, "0")
  return (
    `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}` +
    `T${pad(d.getHours())}:${pad(d.getMinutes())}`
  )
}

export function coerceFixtureValues(values: FixtureFormValues): CoercedFixtureValues {
  return {
    homeId: values.home_participant_id,
    awayId: values.away_participant_id,
    scheduledAt: localToRFC3339(values.scheduled_at) ?? new Date(values.scheduled_at).toISOString(),
    venue: values.venue || undefined,
    roundName: values.round_name || undefined,
    roundNumber: values.round_number ? Number(values.round_number) : undefined,
    matchNumber: values.match_number ? Number(values.match_number) : undefined,
  }
}

// ── Component ─────────────────────────────────────────────────────────────────

interface FixtureFormProps {
  participants: FixtureParticipant[]
  participantNoun: "team" | "player"
  isEdit?: boolean
  isPending: boolean
  defaultValues?: Partial<FixtureFormValues>
  onSubmit: (values: FixtureFormValues) => void
  onCancel?: () => void
}

export function FixtureForm({
  participants,
  participantNoun,
  isEdit = false,
  isPending,
  defaultValues,
  onSubmit,
  onCancel,
}: FixtureFormProps) {
  const form = useForm<FixtureFormValues>({
    resolver: zodResolver(schema),
    mode: "onBlur",
    defaultValues: {
      home_participant_id: "",
      away_participant_id: "",
      scheduled_at: "",
      venue: "",
      round_name: "",
      round_number: "",
      match_number: "",
      ...defaultValues,
    },
  })

  useEffect(() => {
    if (defaultValues) {
      form.reset({
        home_participant_id: "",
        away_participant_id: "",
        scheduled_at: "",
        venue: "",
        round_name: "",
        round_number: "",
        match_number: "",
        ...defaultValues,
      })
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps -- reset only on incoming defaults
  }, [defaultValues])

  const homeId = useWatch({ control: form.control, name: "home_participant_id" })
  const awayId = useWatch({ control: form.control, name: "away_participant_id" })

  // Each side cannot pick the participant the other side has selected — a match
  // against oneself is rejected by the backend (ErrDuplicateParticipants).
  const homeOptions: SelectOption[] = participants.map((p) => ({
    value: p.id,
    label: p.name,
    disabled: p.id === awayId,
  }))
  const awayOptions: SelectOption[] = participants.map((p) => ({
    value: p.id,
    label: p.name,
    disabled: p.id === homeId,
  }))

  const isDirty = form.formState.isDirty
  const noParticipants = participants.length < 2

  return (
    <form onSubmit={form.handleSubmit(onSubmit)} noValidate className="space-y-6">
      <fieldset className="space-y-4" disabled={noParticipants}>
        <legend className="text-sm font-medium text-foreground">Participants</legend>
        <div className="grid gap-4 sm:grid-cols-2">
          <FormSelect
            control={form.control}
            name="home_participant_id"
            label={`Home ${participantNoun}`}
            placeholder={`Select ${participantNoun}…`}
            options={homeOptions}
            required
          />
          <FormSelect
            control={form.control}
            name="away_participant_id"
            label={`Away ${participantNoun}`}
            placeholder={`Select ${participantNoun}…`}
            options={awayOptions}
            required
          />
        </div>
        {noParticipants && (
          <p className="text-xs text-destructive" role="alert">
            At least two approved {participantNoun}s are required to create a fixture.
          </p>
        )}
      </fieldset>

      <Separator />

      <fieldset className="space-y-4">
        <legend className="text-sm font-medium text-foreground">Schedule</legend>
        <FormDatePicker
          control={form.control}
          name="scheduled_at"
          label="Scheduled at"
          required
        />
        <FormField
          control={form.control}
          name="venue"
          label="Venue"
          placeholder="e.g. Court 1, DY Patil Stadium"
        />
      </fieldset>

      <Separator />

      <fieldset className="space-y-4">
        <legend className="text-sm font-medium text-foreground">Position (optional)</legend>
        <div className="grid gap-4 sm:grid-cols-3">
          <FormField
            control={form.control}
            name="round_number"
            label="Round number"
            type="number"
            placeholder="e.g. 1"
            min={1}
            max={1000}
          />
          <FormField
            control={form.control}
            name="round_name"
            label="Round name"
            placeholder="e.g. Quarter-final"
          />
          <FormField
            control={form.control}
            name="match_number"
            label="Match number"
            type="number"
            placeholder="e.g. 3"
            min={1}
            max={10000}
          />
        </div>
      </fieldset>

      <div className="flex items-center gap-3 pt-1">
        <Button
          type="submit"
          disabled={isPending || noParticipants || (isEdit && !isDirty)}
          className="gap-2"
        >
          {isPending && <Loader2Icon className="size-3.5 animate-spin" />}
          {isEdit ? "Save changes" : "Create fixture"}
        </Button>
        {onCancel && (
          <Button type="button" variant="ghost" onClick={onCancel} disabled={isPending}>
            Cancel
          </Button>
        )}
        {isEdit && isDirty && !isPending && (
          <span className="text-xs text-muted-foreground">Unsaved changes</span>
        )}
      </div>
    </form>
  )
}
