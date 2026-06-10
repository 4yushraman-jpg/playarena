"use client"

import { useState } from "react"
import { useForm, useWatch } from "react-hook-form"
import { zodResolver } from "@hookform/resolvers/zod"
import { z } from "zod"
import { useMutation } from "@tanstack/react-query"
import { Loader2Icon, ShieldCheckIcon, EyeIcon, EyeOffIcon, CheckIcon } from "lucide-react"
import { toast } from "sonner"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Separator } from "@/components/ui/separator"
import { usersApi } from "@/lib/api/users"
import { extractApiError } from "@/lib/api-error"
import { useAuthStore, selectUserId } from "@/stores/auth.store"
import { cn } from "@/lib/utils"

// ── Password strength ─────────────────────────────────────────────────────────

interface StrengthRule {
  label: string
  test: (pw: string) => boolean
}

const STRENGTH_RULES: StrengthRule[] = [
  { label: "At least 8 characters", test: (pw) => pw.length >= 8 },
  { label: "Uppercase letter", test: (pw) => /[A-Z]/.test(pw) },
  { label: "Lowercase letter", test: (pw) => /[a-z]/.test(pw) },
  { label: "Number", test: (pw) => /\d/.test(pw) },
]

function getStrength(pw: string): number {
  return STRENGTH_RULES.filter((r) => r.test(pw)).length
}

const STRENGTH_LABELS = ["", "Weak", "Fair", "Good", "Strong"]
const STRENGTH_COLORS = [
  "",
  "bg-destructive",
  "bg-[--color-warning]",
  "bg-[--color-info]",
  "bg-[--color-success]",
]

// ── Schema ────────────────────────────────────────────────────────────────────

const schema = z
  .object({
    current_password: z.string().min(1, "Current password is required"),
    new_password: z
      .string()
      .min(8, "Password must be at least 8 characters")
      .regex(/[A-Z]/, "Must contain an uppercase letter")
      .regex(/[a-z]/, "Must contain a lowercase letter")
      .regex(/\d/, "Must contain a number"),
    confirm_password: z.string().min(1, "Please confirm your new password"),
  })
  .refine((d) => d.new_password === d.confirm_password, {
    path: ["confirm_password"],
    message: "Passwords do not match",
  })

type FormValues = z.infer<typeof schema>

// ── Component ─────────────────────────────────────────────────────────────────

export default function SecuritySettingsPage() {
  const userId = useAuthStore(selectUserId)
  const [showCurrent, setShowCurrent] = useState(false)
  const [showNew, setShowNew] = useState(false)
  const [showConfirm, setShowConfirm] = useState(false)

  const form = useForm<FormValues>({
    resolver: zodResolver(schema),
    defaultValues: { current_password: "", new_password: "", confirm_password: "" },
    mode: "onBlur",
  })

  const newPassword = useWatch({ control: form.control, name: "new_password" }) ?? ""
  const strength = getStrength(newPassword)

  const mutation = useMutation({
    mutationFn: (data: FormValues) =>
      usersApi.changePassword(userId!, {
        current_password: data.current_password,
        new_password: data.new_password,
      }),
    onSuccess: () => {
      form.reset()
      toast.success("Password updated successfully")
    },
    onError: (err) => {
      const msg = extractApiError(err)
      if (msg?.toLowerCase().includes("incorrect") || msg?.toLowerCase().includes("current")) {
        form.setError("current_password", { message: msg })
      } else {
        toast.error(msg ?? "Failed to update password")
      }
    },
  })

  return (
    <div className="space-y-6">
      <div>
        <h2 className="text-lg font-semibold">Security</h2>
        <p className="text-sm text-muted-foreground">
          Update your password to keep your account secure.
        </p>
      </div>

      <Separator />

      <form
        onSubmit={form.handleSubmit((data) => mutation.mutate(data))}
        noValidate
        className="max-w-md space-y-5"
      >
        {/* Current password */}
        <PasswordField
          id="current_password"
          label="Current password"
          show={showCurrent}
          onToggle={() => setShowCurrent((v) => !v)}
          registration={form.register("current_password")}
          error={form.formState.errors.current_password?.message}
          autoComplete="current-password"
        />

        {/* New password + strength */}
        <div className="space-y-2">
          <PasswordField
            id="new_password"
            label="New password"
            show={showNew}
            onToggle={() => setShowNew((v) => !v)}
            registration={form.register("new_password")}
            error={form.formState.errors.new_password?.message}
            autoComplete="new-password"
          />

          {/* Strength bar */}
          {newPassword.length > 0 && (
            <div className="space-y-1.5" aria-live="polite" aria-label={`Password strength: ${STRENGTH_LABELS[strength]}`}>
              <div className="flex gap-1">
                {[1, 2, 3, 4].map((i) => (
                  <div
                    key={i}
                    className={cn(
                      "h-1 flex-1 rounded-full transition-all",
                      i <= strength ? STRENGTH_COLORS[strength] : "bg-muted",
                    )}
                  />
                ))}
              </div>
              <p className="text-xs text-muted-foreground">{STRENGTH_LABELS[strength]}</p>
              <ul className="space-y-0.5" aria-label="Password requirements">
                {STRENGTH_RULES.map((rule) => {
                  const passed = rule.test(newPassword)
                  return (
                    <li
                      key={rule.label}
                      className={cn(
                        "flex items-center gap-1.5 text-xs",
                        passed ? "text-[--color-success]" : "text-muted-foreground",
                      )}
                    >
                      <CheckIcon
                        className={cn(
                          "size-3 shrink-0",
                          passed ? "opacity-100" : "opacity-30",
                        )}
                        aria-hidden="true"
                      />
                      {rule.label}
                    </li>
                  )
                })}
              </ul>
            </div>
          )}
        </div>

        {/* Confirm password */}
        <PasswordField
          id="confirm_password"
          label="Confirm new password"
          show={showConfirm}
          onToggle={() => setShowConfirm((v) => !v)}
          registration={form.register("confirm_password")}
          error={form.formState.errors.confirm_password?.message}
          autoComplete="new-password"
        />

        <Button
          type="submit"
          disabled={mutation.isPending}
          className="gap-2"
        >
          {mutation.isPending ? (
            <Loader2Icon className="size-3.5 animate-spin" />
          ) : (
            <ShieldCheckIcon className="size-3.5" />
          )}
          Update password
        </Button>
      </form>
    </div>
  )
}

// ── PasswordField helper ──────────────────────────────────────────────────────

interface PasswordFieldProps {
  id: string
  label: string
  show: boolean
  onToggle: () => void
  registration: ReturnType<ReturnType<typeof useForm<FormValues>>["register"]>
  error?: string
  autoComplete?: string
}

function PasswordField({
  id,
  label,
  show,
  onToggle,
  registration,
  error,
  autoComplete,
}: PasswordFieldProps) {
  return (
    <div className="space-y-1.5">
      <Label
        htmlFor={id}
        className="after:ml-0.5 after:text-destructive after:content-['*']"
      >
        {label}
      </Label>
      <div className="relative">
        <Input
          id={id}
          type={show ? "text" : "password"}
          autoComplete={autoComplete}
          aria-invalid={!!error}
          aria-describedby={error ? `${id}-error` : undefined}
          className="pr-10"
          {...registration}
        />
        <button
          type="button"
          onClick={onToggle}
          className="absolute right-2.5 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          aria-label={show ? "Hide password" : "Show password"}
        >
          {show ? <EyeOffIcon className="size-4" /> : <EyeIcon className="size-4" />}
        </button>
      </div>
      {error && (
        <p id={`${id}-error`} role="alert" className="text-xs text-destructive">
          {error}
        </p>
      )}
    </div>
  )
}
