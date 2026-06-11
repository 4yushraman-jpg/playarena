"use client"

import { useEffect } from "react"
import { useForm } from "react-hook-form"
import { zodResolver } from "@hookform/resolvers/zod"
import { z } from "zod"
import { Loader2Icon } from "lucide-react"
import { Button } from "@/components/ui/button"
import {
  FormField,
  FormTextarea,
  FormSelect,
  FormDatePicker,
} from "@/components/ui/form-field"
import { Separator } from "@/components/ui/separator"
import type {
  Tournament,
  TournamentFormat,
  ParticipantType,
} from "@/types/api/tournaments"

// ── Schema ─────────────────────────────────────────────────────────────────────

const DECIMAL_RE = /^\d+(\.\d{1,2})?$/
const ISO2_RE = /^[A-Z]{2}$/
const ISO4217_RE = /^[A-Z]{3}$/

const FORMAT_OPTIONS = [
  { value: "league", label: "League" },
  { value: "knockout", label: "Knockout" },
  { value: "group_knockout", label: "Group + Knockout" },
  { value: "round_robin", label: "Round Robin" },
  { value: "double_elimination", label: "Double Elimination" },
]

const PARTICIPANT_OPTIONS = [
  { value: "team", label: "Team" },
  { value: "individual", label: "Individual" },
]

const schema = z
  .object({
    name: z.string().min(2, "Name must be at least 2 characters").max(255, "Name is too long"),
    sport: z.string().min(2, "Sport is required").max(100, "Sport name is too long"),
    format: z.enum(["league", "knockout", "group_knockout", "round_robin", "double_elimination"]),
    participant_type: z.enum(["team", "individual"]).optional(),
    description: z.string().max(2000, "Description is too long").optional().or(z.literal("")),
    prize_pool: z
      .string()
      .refine((v) => v === "" || DECIMAL_RE.test(v), { message: "Must be a number like 10000.00" })
      .optional()
      .or(z.literal("")),
    currency: z
      .string()
      .refine((v) => v === "" || ISO4217_RE.test(v), { message: "Must be 3-letter code e.g. INR" })
      .optional()
      .or(z.literal("")),
    max_participants: z
      .string()
      .refine(
        (v) => v === "" || (Number.isInteger(Number(v)) && Number(v) >= 2 && Number(v) <= 1024),
        { message: "Must be a whole number between 2 and 1024" },
      )
      .optional()
      .or(z.literal("")),
    min_participants: z
      .string()
      .refine(
        (v) => v === "" || (Number.isInteger(Number(v)) && Number(v) >= 2 && Number(v) <= 1024),
        { message: "Must be a whole number between 2 and 1024" },
      )
      .optional()
      .or(z.literal("")),
    registration_opens_at: z.string().optional().or(z.literal("")),
    registration_closes_at: z.string().optional().or(z.literal("")),
    starts_at: z.string().optional().or(z.literal("")),
    ends_at: z.string().optional().or(z.literal("")),
    venue: z.string().max(200, "Venue is too long").optional().or(z.literal("")),
    city: z.string().max(100, "City is too long").optional().or(z.literal("")),
    country: z
      .string()
      .refine((v) => v === "" || ISO2_RE.test(v), { message: "Must be 2-letter code e.g. IN" })
      .optional()
      .or(z.literal("")),
    rules: z.string().max(10000, "Rules are too long").optional().or(z.literal("")),
  })
  .refine(
    (d) => {
      if (d.max_participants && d.min_participants) {
        return Number(d.min_participants) <= Number(d.max_participants)
      }
      return true
    },
    { message: "Min participants cannot exceed max", path: ["min_participants"] },
  )

export type TournamentFormValues = z.infer<typeof schema>

// ── Coerce helpers ─────────────────────────────────────────────────────────────

// Converts datetime-local string "YYYY-MM-DDTHH:mm" → RFC3339 UTC string
function localToRFC3339(local: string | undefined): string | undefined {
  if (!local) return undefined
  const d = new Date(local)
  if (isNaN(d.getTime())) return undefined
  return d.toISOString()
}

// Explicitly typed so callers can build create/update requests without enum
// casts — the schema enums are declared to match the API unions exactly.
export interface CoercedTournamentValues {
  name: string
  sport: string
  format: TournamentFormat
  participant_type?: ParticipantType
  description?: string
  prize_pool?: string
  currency?: string
  max_participants?: number
  min_participants?: number
  registration_opens_at?: string
  registration_closes_at?: string
  starts_at?: string
  ends_at?: string
  venue?: string
  city?: string
  country?: string
  rules?: string
}

export function coerceTournamentFormValues(values: TournamentFormValues): CoercedTournamentValues {
  return {
    name: values.name,
    sport: values.sport,
    format: values.format,
    participant_type: values.participant_type,
    description: values.description || undefined,
    prize_pool: values.prize_pool || undefined,
    currency: values.currency || undefined,
    max_participants: values.max_participants ? Number(values.max_participants) : undefined,
    min_participants: values.min_participants ? Number(values.min_participants) : undefined,
    registration_opens_at: localToRFC3339(values.registration_opens_at),
    registration_closes_at: localToRFC3339(values.registration_closes_at),
    starts_at: localToRFC3339(values.starts_at),
    ends_at: localToRFC3339(values.ends_at),
    venue: values.venue || undefined,
    city: values.city || undefined,
    country: values.country || undefined,
    rules: values.rules || undefined,
  }
}

// Converts RFC3339 UTC string → "YYYY-MM-DDTHH:mm" for datetime-local input
function rfc3339ToLocal(iso: string | null | undefined): string {
  if (!iso) return ""
  const d = new Date(iso)
  if (isNaN(d.getTime())) return ""
  // Pad each part to the format datetime-local expects
  const pad = (n: number) => String(n).padStart(2, "0")
  return (
    `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}` +
    `T${pad(d.getHours())}:${pad(d.getMinutes())}`
  )
}

// ── Component ─────────────────────────────────────────────────────────────────

interface TournamentFormProps {
  defaultValues?: Partial<Tournament>
  isEdit?: boolean
  isPending: boolean
  onSubmit: (values: TournamentFormValues) => void
  onCancel?: () => void
}

export function TournamentForm({
  defaultValues,
  isEdit = false,
  isPending,
  onSubmit,
  onCancel,
}: TournamentFormProps) {
  const form = useForm<TournamentFormValues>({
    resolver: zodResolver(schema),
    mode: "onBlur",
    defaultValues: {
      name: "",
      sport: "",
      format: "knockout",
      participant_type: "team",
      description: "",
      prize_pool: "",
      currency: "INR",
      max_participants: "",
      min_participants: "",
      registration_opens_at: "",
      registration_closes_at: "",
      starts_at: "",
      ends_at: "",
      venue: "",
      city: "",
      country: "IN",
      rules: "",
    },
  })

  useEffect(() => {
    if (defaultValues) {
      form.reset({
        name: defaultValues.name ?? "",
        sport: defaultValues.sport ?? "",
        format: defaultValues.format ?? "knockout",
        participant_type: defaultValues.participant_type ?? "team",
        description: defaultValues.description ?? "",
        prize_pool: defaultValues.prize_pool ?? "",
        currency: defaultValues.currency ?? "INR",
        max_participants:
          defaultValues.max_participants != null
            ? String(defaultValues.max_participants)
            : "",
        min_participants:
          defaultValues.min_participants != null
            ? String(defaultValues.min_participants)
            : "",
        registration_opens_at: rfc3339ToLocal(defaultValues.registration_opens_at),
        registration_closes_at: rfc3339ToLocal(defaultValues.registration_closes_at),
        starts_at: rfc3339ToLocal(defaultValues.starts_at),
        ends_at: rfc3339ToLocal(defaultValues.ends_at),
        venue: defaultValues.venue ?? "",
        city: defaultValues.city ?? "",
        country: defaultValues.country ?? "IN",
        rules: defaultValues.rules ?? "",
      })
    }
  }, [defaultValues, form])

  const isDirty = form.formState.isDirty

  return (
    <form onSubmit={form.handleSubmit(onSubmit)} noValidate className="space-y-6">
      {/* ── Section: Identity ─────────────────────────────────── */}
      <fieldset className="space-y-4">
        <legend className="text-sm font-medium text-foreground">Tournament details</legend>
        <FormField
          control={form.control}
          name="name"
          label="Name"
          placeholder="e.g. Summer Knockout 2025"
          required
        />
        <div className="grid gap-4 sm:grid-cols-2">
          <FormField
            control={form.control}
            name="sport"
            label="Sport"
            placeholder="e.g. Football, Cricket, Chess"
            required
          />
          <FormSelect
            control={form.control}
            name="format"
            label="Format"
            options={FORMAT_OPTIONS}
            required
          />
        </div>
        <FormSelect
          control={form.control}
          name="participant_type"
          label="Participant type"
          options={PARTICIPANT_OPTIONS}
        />
        <FormTextarea
          control={form.control}
          name="description"
          label="Description"
          placeholder="Brief description visible to participants…"
          rows={3}
        />
      </fieldset>

      <Separator />

      {/* ── Section: Schedule ─────────────────────────────────── */}
      <fieldset className="space-y-4">
        <legend className="text-sm font-medium text-foreground">Schedule</legend>
        <div className="grid gap-4 sm:grid-cols-2">
          <FormDatePicker
            control={form.control}
            name="registration_opens_at"
            label="Registration opens"
            description="Leave blank to open immediately on publish"
          />
          <FormDatePicker
            control={form.control}
            name="registration_closes_at"
            label="Registration closes"
          />
        </div>
        <div className="grid gap-4 sm:grid-cols-2">
          <FormDatePicker
            control={form.control}
            name="starts_at"
            label="Tournament starts"
          />
          <FormDatePicker
            control={form.control}
            name="ends_at"
            label="Tournament ends"
          />
        </div>
      </fieldset>

      <Separator />

      {/* ── Section: Capacity & Prize ─────────────────────────── */}
      <fieldset className="space-y-4">
        <legend className="text-sm font-medium text-foreground">Capacity &amp; prize</legend>
        <div className="grid gap-4 sm:grid-cols-2">
          <FormField
            control={form.control}
            name="min_participants"
            label="Min participants"
            type="number"
            placeholder="e.g. 4"
            min={2}
            max={1024}
          />
          <FormField
            control={form.control}
            name="max_participants"
            label="Max participants"
            type="number"
            placeholder="e.g. 16"
            min={2}
            max={1024}
          />
        </div>
        <div className="grid gap-4 sm:grid-cols-2">
          <FormField
            control={form.control}
            name="prize_pool"
            label="Prize pool"
            placeholder="e.g. 50000.00"
          />
          <FormField
            control={form.control}
            name="currency"
            label="Currency (ISO 4217)"
            placeholder="INR"
            maxLength={3}
          />
        </div>
      </fieldset>

      <Separator />

      {/* ── Section: Location ─────────────────────────────────── */}
      <fieldset className="space-y-4">
        <legend className="text-sm font-medium text-foreground">Location</legend>
        <FormField
          control={form.control}
          name="venue"
          label="Venue"
          placeholder="e.g. DY Patil Stadium"
        />
        <div className="grid gap-4 sm:grid-cols-2">
          <FormField
            control={form.control}
            name="city"
            label="City"
            placeholder="e.g. Mumbai"
          />
          <FormField
            control={form.control}
            name="country"
            label="Country (ISO 3166-1 alpha-2)"
            placeholder="IN"
            maxLength={2}
          />
        </div>
      </fieldset>

      <Separator />

      {/* ── Section: Rules ────────────────────────────────────── */}
      <fieldset className="space-y-4">
        <legend className="text-sm font-medium text-foreground">Rules</legend>
        <FormTextarea
          control={form.control}
          name="rules"
          label="Tournament rules"
          placeholder="Describe the rules, scoring system, tiebreakers…"
          rows={5}
        />
      </fieldset>

      {/* ── Actions ───────────────────────────────────────────── */}
      <div className="flex items-center gap-3 pt-2">
        <Button type="submit" disabled={isPending || (isEdit && !isDirty)} className="gap-2">
          {isPending && <Loader2Icon className="size-3.5 animate-spin" />}
          {isEdit ? "Save changes" : "Create tournament"}
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
