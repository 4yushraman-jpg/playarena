"use client"

/**
 * RHF Controller wrappers for shadcn primitives.
 *
 * Each component takes `control`, `name`, `label`, optional `description` and
 * `required`, and renders a fully accessible field with error state wired via
 * the RHF FieldState. This pattern means form authors never touch Controller
 * directly for standard inputs.
 *
 * Usage:
 *   <FormField control={form.control} name="email" label="Email" type="email" required />
 *   <FormSelect control={form.control} name="status" label="Status" options={statusOptions} />
 */

import * as React from "react"
import { Controller, type Control, type FieldPath, type FieldValues } from "react-hook-form"
import { cn } from "@/lib/utils"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"

// ── Shared field layout ───────────────────────────────────────────────────────

interface FieldWrapperProps {
  id: string
  label: string
  description?: string
  error?: string
  required?: boolean
  children: React.ReactNode
}

function FieldWrapper({ id, label, description, error, required, children }: FieldWrapperProps) {
  return (
    <div className="space-y-1.5">
      <Label htmlFor={id} className={cn(required && "after:ml-0.5 after:text-destructive after:content-['*']")}>
        {label}
      </Label>
      {children}
      {description && !error && (
        <p className="text-xs text-muted-foreground">{description}</p>
      )}
      {error && (
        <p id={`${id}-error`} role="alert" className="text-xs text-destructive">
          {error}
        </p>
      )}
    </div>
  )
}

// ── FormField — text/number/email/password/etc. ───────────────────────────────

interface FormFieldProps<
  TFieldValues extends FieldValues = FieldValues,
  TName extends FieldPath<TFieldValues> = FieldPath<TFieldValues>,
> extends Omit<React.ComponentProps<"input">, "name" | "id"> {
  control: Control<TFieldValues>
  name: TName
  label: string
  description?: string
}

export function FormField<
  TFieldValues extends FieldValues = FieldValues,
  TName extends FieldPath<TFieldValues> = FieldPath<TFieldValues>,
>({
  control,
  name,
  label,
  description,
  required,
  className,
  ...inputProps
}: FormFieldProps<TFieldValues, TName>) {
  const id = `field-${name}`

  return (
    <Controller
      control={control}
      name={name}
      render={({ field, fieldState }) => (
        <FieldWrapper
          id={id}
          label={label}
          description={description}
          error={fieldState.error?.message}
          required={required}
        >
          <Input
            id={id}
            {...inputProps}
            {...field}
            value={field.value ?? ""}
            required={required}
            aria-invalid={fieldState.invalid}
            aria-describedby={fieldState.error ? `${id}-error` : undefined}
            className={className}
          />
        </FieldWrapper>
      )}
    />
  )
}

// ── FormTextarea ──────────────────────────────────────────────────────────────

interface FormTextareaProps<
  TFieldValues extends FieldValues = FieldValues,
  TName extends FieldPath<TFieldValues> = FieldPath<TFieldValues>,
> extends Omit<React.ComponentProps<"textarea">, "name" | "id"> {
  control: Control<TFieldValues>
  name: TName
  label: string
  description?: string
}

export function FormTextarea<
  TFieldValues extends FieldValues = FieldValues,
  TName extends FieldPath<TFieldValues> = FieldPath<TFieldValues>,
>({
  control,
  name,
  label,
  description,
  required,
  className,
  ...textareaProps
}: FormTextareaProps<TFieldValues, TName>) {
  const id = `field-${name}`

  return (
    <Controller
      control={control}
      name={name}
      render={({ field, fieldState }) => (
        <FieldWrapper
          id={id}
          label={label}
          description={description}
          error={fieldState.error?.message}
          required={required}
        >
          <textarea
            id={id}
            {...textareaProps}
            {...field}
            value={field.value ?? ""}
            required={required}
            aria-invalid={fieldState.invalid}
            aria-describedby={fieldState.error ? `${id}-error` : undefined}
            className={cn(
              "w-full min-h-20 rounded-lg border border-input bg-transparent px-2.5 py-2 text-sm outline-none placeholder:text-muted-foreground focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50 disabled:pointer-events-none disabled:opacity-50 aria-invalid:border-destructive aria-invalid:ring-3 aria-invalid:ring-destructive/20 dark:bg-input/30 resize-y",
              className,
            )}
          />
        </FieldWrapper>
      )}
    />
  )
}

// ── FormSelect ────────────────────────────────────────────────────────────────

export interface SelectOption {
  value: string
  label: string
  disabled?: boolean
}

interface FormSelectProps<
  TFieldValues extends FieldValues = FieldValues,
  TName extends FieldPath<TFieldValues> = FieldPath<TFieldValues>,
> {
  control: Control<TFieldValues>
  name: TName
  label: string
  description?: string
  required?: boolean
  placeholder?: string
  options: SelectOption[]
  disabled?: boolean
  className?: string
}

export function FormSelect<
  TFieldValues extends FieldValues = FieldValues,
  TName extends FieldPath<TFieldValues> = FieldPath<TFieldValues>,
>({
  control,
  name,
  label,
  description,
  required,
  placeholder = "Select…",
  options,
  disabled,
  className,
}: FormSelectProps<TFieldValues, TName>) {
  const id = `field-${name}`

  return (
    <Controller
      control={control}
      name={name}
      render={({ field, fieldState }) => (
        <FieldWrapper
          id={id}
          label={label}
          description={description}
          error={fieldState.error?.message}
          required={required}
        >
          <Select
            value={field.value ?? ""}
            onValueChange={field.onChange}
            disabled={disabled}
          >
            <SelectTrigger
              id={id}
              className={cn("w-full", fieldState.invalid && "border-destructive ring-destructive/20", className)}
              aria-invalid={fieldState.invalid}
              aria-describedby={fieldState.error ? `${id}-error` : undefined}
            >
              <SelectValue placeholder={placeholder} />
            </SelectTrigger>
            <SelectContent>
              {options.map((opt) => (
                <SelectItem key={opt.value} value={opt.value} disabled={opt.disabled}>
                  {opt.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </FieldWrapper>
      )}
    />
  )
}

// ── FormDatePicker — native date input (no external dep) ──────────────────────

interface FormDatePickerProps<
  TFieldValues extends FieldValues = FieldValues,
  TName extends FieldPath<TFieldValues> = FieldPath<TFieldValues>,
> {
  control: Control<TFieldValues>
  name: TName
  label: string
  description?: string
  required?: boolean
  min?: string
  max?: string
  disabled?: boolean
  className?: string
}

export function FormDatePicker<
  TFieldValues extends FieldValues = FieldValues,
  TName extends FieldPath<TFieldValues> = FieldPath<TFieldValues>,
>({
  control,
  name,
  label,
  description,
  required,
  min,
  max,
  disabled,
  className,
}: FormDatePickerProps<TFieldValues, TName>) {
  const id = `field-${name}`

  return (
    <Controller
      control={control}
      name={name}
      render={({ field, fieldState }) => (
        <FieldWrapper
          id={id}
          label={label}
          description={description}
          error={fieldState.error?.message}
          required={required}
        >
          <Input
            id={id}
            type="datetime-local"
            min={min}
            max={max}
            disabled={disabled}
            required={required}
            aria-invalid={fieldState.invalid}
            aria-describedby={fieldState.error ? `${id}-error` : undefined}
            className={cn("w-full", className)}
            {...field}
            value={field.value ?? ""}
          />
        </FieldWrapper>
      )}
    />
  )
}
