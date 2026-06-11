"use client"

import { useEffect } from "react"
import { useForm } from "react-hook-form"
import { zodResolver } from "@hookform/resolvers/zod"
import { z } from "zod"
import { Loader2Icon } from "lucide-react"
import { Button } from "@/components/ui/button"
import { FormField, FormTextarea, FormSelect } from "@/components/ui/form-field"
import type { Player } from "@/types/api/players"

// ── Schema ─────────────────────────────────────────────────────────────────────

const DOMINANT_HAND_OPTIONS = [
  { value: "left", label: "Left" },
  { value: "right", label: "Right" },
  { value: "ambidextrous", label: "Ambidextrous" },
]

const STATUS_OPTIONS = [
  { value: "active", label: "Active" },
  { value: "inactive", label: "Inactive" },
  { value: "injured", label: "Injured" },
  { value: "suspended", label: "Suspended" },
  { value: "retired", label: "Retired" },
]

// Keep numeric fields as strings — coerce to number in onSubmit
const schema = z.object({
  display_name: z.string().min(1, "Display name is required").max(100, "Name is too long"),
  jersey_number: z.string().max(10, "Jersey number is too long").optional().or(z.literal("")),
  position: z.string().max(50, "Position is too long").optional().or(z.literal("")),
  height_cm: z
    .string()
    .refine(
      (v) => v === "" || (!isNaN(Number(v)) && Number(v) >= 51 && Number(v) <= 299),
      { message: "Height must be between 51 and 299 cm" },
    )
    .optional()
    .or(z.literal("")),
  weight_kg: z
    .string()
    .refine(
      (v) => v === "" || (!isNaN(Number(v)) && Number(v) >= 21 && Number(v) <= 299),
      { message: "Weight must be between 21 and 299 kg" },
    )
    .optional()
    .or(z.literal("")),
  dominant_hand: z.enum(["left", "right", "ambidextrous"]).optional().or(z.literal("")),
  date_of_birth: z.string().optional().or(z.literal("")),
  nationality: z
    .string()
    .refine((v) => v === "" || v.length === 2, { message: "Must be a 2-letter country code (e.g. IN, US)" })
    .optional()
    .or(z.literal("")),
  bio: z.string().max(1000, "Bio is too long").optional().or(z.literal("")),
  status: z.enum(["active", "inactive", "injured", "suspended", "retired"]).optional(),
})

export type PlayerFormValues = z.infer<typeof schema>

// ── Coerce helpers ─────────────────────────────────────────────────────────────

export function coercePlayerFormValues(values: PlayerFormValues) {
  return {
    display_name: values.display_name,
    jersey_number: values.jersey_number || undefined,
    position: values.position || undefined,
    height_cm: values.height_cm && values.height_cm !== "" ? Number(values.height_cm) : undefined,
    weight_kg: values.weight_kg && values.weight_kg !== "" ? Number(values.weight_kg) : undefined,
    dominant_hand: (values.dominant_hand as string) || undefined,
    date_of_birth: values.date_of_birth || undefined,
    nationality: values.nationality ? values.nationality.toUpperCase() : undefined,
    bio: values.bio || undefined,
    status: values.status,
  }
}

// ── Component ─────────────────────────────────────────────────────────────────

interface PlayerFormProps {
  defaultValues?: Partial<Player>
  isEdit?: boolean
  isPending: boolean
  onSubmit: (values: PlayerFormValues) => void
  onCancel?: () => void
}

export function PlayerForm({
  defaultValues,
  isEdit = false,
  isPending,
  onSubmit,
  onCancel,
}: PlayerFormProps) {
  const form = useForm<PlayerFormValues>({
    resolver: zodResolver(schema),
    mode: "onBlur",
    defaultValues: {
      display_name: "",
      jersey_number: "",
      position: "",
      height_cm: "",
      weight_kg: "",
      dominant_hand: "",
      date_of_birth: "",
      nationality: "",
      bio: "",
      status: "active",
    },
  })

  useEffect(() => {
    if (defaultValues) {
      form.reset({
        display_name: defaultValues.display_name ?? "",
        jersey_number: defaultValues.jersey_number ?? "",
        position: defaultValues.position ?? "",
        height_cm: defaultValues.height_cm != null ? String(defaultValues.height_cm) : "",
        weight_kg: defaultValues.weight_kg != null ? String(defaultValues.weight_kg) : "",
        dominant_hand: (defaultValues.dominant_hand as PlayerFormValues["dominant_hand"]) ?? "",
        date_of_birth: defaultValues.date_of_birth ?? "",
        nationality: defaultValues.nationality ?? "",
        bio: defaultValues.bio ?? "",
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
          name="display_name"
          label="Display name"
          placeholder="Full name"
          required
        />
        <FormField
          control={form.control}
          name="jersey_number"
          label="Jersey number"
          placeholder="e.g. 10"
        />
      </div>

      <div className="grid gap-4 sm:grid-cols-2">
        <FormField
          control={form.control}
          name="position"
          label="Position"
          placeholder="e.g. Forward, Goalkeeper"
        />
        <FormSelect
          control={form.control}
          name="dominant_hand"
          label="Dominant hand"
          placeholder="Select…"
          options={DOMINANT_HAND_OPTIONS}
        />
      </div>

      {/* Physical */}
      <div className="grid gap-4 sm:grid-cols-2">
        <FormField
          control={form.control}
          name="height_cm"
          label="Height (cm)"
          type="number"
          placeholder="e.g. 180"
          min={51}
          max={299}
        />
        <FormField
          control={form.control}
          name="weight_kg"
          label="Weight (kg)"
          type="number"
          placeholder="e.g. 75"
          min={21}
          max={299}
        />
      </div>

      {/* Background */}
      <div className="grid gap-4 sm:grid-cols-2">
        <FormField
          control={form.control}
          name="date_of_birth"
          label="Date of birth"
          type="date"
          max={new Date().toISOString().split("T")[0]}
        />
        <FormField
          control={form.control}
          name="nationality"
          label="Nationality (ISO 2-letter)"
          placeholder="e.g. IN, US"
          maxLength={2}
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

      <FormTextarea
        control={form.control}
        name="bio"
        label="Bio"
        placeholder="Short description of the player…"
        rows={3}
      />

      {/* Actions */}
      <div className="flex items-center gap-3 pt-2">
        <Button type="submit" disabled={isPending || (isEdit && !isDirty)} className="gap-2">
          {isPending && <Loader2Icon className="size-3.5 animate-spin" />}
          {isEdit ? "Save changes" : "Create player"}
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
