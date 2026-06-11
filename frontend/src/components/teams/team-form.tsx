"use client"

import { useEffect } from "react"
import { useForm } from "react-hook-form"
import { zodResolver } from "@hookform/resolvers/zod"
import { z } from "zod"
import { Loader2Icon } from "lucide-react"
import { Button } from "@/components/ui/button"
import { FormField, FormTextarea, FormSelect } from "@/components/ui/form-field"
import type { Team } from "@/types/api/teams"

// ── Schema ─────────────────────────────────────────────────────────────────────

const HEX_RE = /^#[0-9A-Fa-f]{6}$/

const STATUS_OPTIONS = [
  { value: "active", label: "Active" },
  { value: "inactive", label: "Inactive" },
  { value: "disbanded", label: "Disbanded" },
]

// Keep numeric fields as strings — coerce to number in onSubmit
const schema = z.object({
  name: z.string().min(1, "Team name is required").max(100, "Name is too long"),
  short_name: z
    .string()
    .refine((v) => v === "" || (v.length >= 2 && v.length <= 10), {
      message: "Short name must be 2–10 characters",
    })
    .optional()
    .or(z.literal("")),
  description: z.string().max(500, "Description is too long").optional().or(z.literal("")),
  home_city: z.string().max(100, "City is too long").optional().or(z.literal("")),
  home_venue: z.string().max(200, "Venue is too long").optional().or(z.literal("")),
  founded_year: z
    .string()
    .refine(
      (v) => {
        if (v === "" || v === undefined) return true
        const n = Number(v)
        return Number.isInteger(n) && n >= 1800 && n <= 2100
      },
      { message: "Founded year must be between 1800 and 2100" },
    )
    .optional()
    .or(z.literal("")),
  primary_color: z
    .string()
    .refine((v) => v === "" || HEX_RE.test(v), { message: "Must be a hex color like #FF0000" })
    .optional()
    .or(z.literal("")),
  secondary_color: z
    .string()
    .refine((v) => v === "" || HEX_RE.test(v), { message: "Must be a hex color like #0000FF" })
    .optional()
    .or(z.literal("")),
  status: z.enum(["active", "inactive", "disbanded"]).optional(),
})

export type TeamFormValues = z.infer<typeof schema>

// ── Coerce helpers ─────────────────────────────────────────────────────────────

export function coerceTeamFormValues(values: TeamFormValues) {
  return {
    name: values.name,
    short_name: values.short_name || undefined,
    description: values.description || undefined,
    home_city: values.home_city || undefined,
    home_venue: values.home_venue || undefined,
    founded_year: values.founded_year && values.founded_year !== "" ? Number(values.founded_year) : undefined,
    primary_color: values.primary_color || undefined,
    secondary_color: values.secondary_color || undefined,
    status: values.status,
  }
}

// ── Component ─────────────────────────────────────────────────────────────────

interface TeamFormProps {
  defaultValues?: Partial<Team>
  isEdit?: boolean
  isPending: boolean
  onSubmit: (values: TeamFormValues) => void
  onCancel?: () => void
}

export function TeamForm({
  defaultValues,
  isEdit = false,
  isPending,
  onSubmit,
  onCancel,
}: TeamFormProps) {
  const form = useForm<TeamFormValues>({
    resolver: zodResolver(schema),
    mode: "onBlur",
    defaultValues: {
      name: "",
      short_name: "",
      description: "",
      home_city: "",
      home_venue: "",
      founded_year: "",
      primary_color: "",
      secondary_color: "",
      status: "active",
    },
  })

  useEffect(() => {
    if (defaultValues) {
      form.reset({
        name: defaultValues.name ?? "",
        short_name: defaultValues.short_name ?? "",
        description: defaultValues.description ?? "",
        home_city: defaultValues.home_city ?? "",
        home_venue: defaultValues.home_venue ?? "",
        founded_year: defaultValues.founded_year != null ? String(defaultValues.founded_year) : "",
        primary_color: defaultValues.primary_color ?? "",
        secondary_color: defaultValues.secondary_color ?? "",
        status: defaultValues.status ?? "active",
      })
    }
  }, [defaultValues, form])

  const isDirty = form.formState.isDirty

  return (
    <form
      onSubmit={form.handleSubmit(onSubmit)}
      noValidate
      className="space-y-5"
    >
      {/* Identity */}
      <div className="grid gap-4 sm:grid-cols-2">
        <FormField
          control={form.control}
          name="name"
          label="Team name"
          placeholder="e.g. Thunder Strikers"
          required
        />
        <FormField
          control={form.control}
          name="short_name"
          label="Short name"
          placeholder="e.g. TS (2–10 chars)"
          maxLength={10}
        />
      </div>

      <FormTextarea
        control={form.control}
        name="description"
        label="Description"
        placeholder="Brief description of the team…"
        rows={3}
      />

      {/* Location */}
      <div className="grid gap-4 sm:grid-cols-2">
        <FormField
          control={form.control}
          name="home_city"
          label="Home city"
          placeholder="e.g. Mumbai"
        />
        <FormField
          control={form.control}
          name="home_venue"
          label="Home venue"
          placeholder="e.g. Wankhede Stadium"
        />
      </div>

      <FormField
        control={form.control}
        name="founded_year"
        label="Founded year"
        type="number"
        placeholder="e.g. 2015"
        min={1800}
        max={2100}
      />

      {/* Colors */}
      <div className="grid gap-4 sm:grid-cols-2">
        <FormField
          control={form.control}
          name="primary_color"
          label="Primary color (hex)"
          placeholder="#FF0000"
          maxLength={7}
        />
        <FormField
          control={form.control}
          name="secondary_color"
          label="Secondary color (hex)"
          placeholder="#0000FF"
          maxLength={7}
        />
      </div>

      {isEdit && (
        <FormSelect
          control={form.control}
          name="status"
          label="Status"
          options={STATUS_OPTIONS}
          required
        />
      )}

      {/* Actions */}
      <div className="flex items-center gap-3 pt-2">
        <Button type="submit" disabled={isPending || (isEdit && !isDirty)} className="gap-2">
          {isPending && <Loader2Icon className="size-3.5 animate-spin" />}
          {isEdit ? "Save changes" : "Create team"}
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
