"use client"

import { Suspense } from "react"
import Link from "next/link"
import { useSearchParams, useRouter } from "next/navigation"
import { useForm } from "react-hook-form"
import { zodResolver } from "@hookform/resolvers/zod"
import { z } from "zod"
import { Button } from "@/components/ui/button"
import { FormField } from "@/components/ui/form-field"
import { authApi } from "@/lib/api/auth"
import { extractApiError } from "@/lib/api-error"

const schema = z
  .object({
    password: z.string().min(8, "Minimum 8 characters").max(72),
    confirm: z.string(),
  })
  .refine((v) => v.password === v.confirm, {
    message: "Passwords do not match",
    path: ["confirm"],
  })

type FormValues = z.infer<typeof schema>

function ResetPasswordContent() {
  const params = useSearchParams()
  const router = useRouter()
  const token = params.get("token") ?? ""

  const form = useForm<FormValues>({
    resolver: zodResolver(schema),
    defaultValues: { password: "", confirm: "" },
  })

  const { handleSubmit, setError, formState: { isSubmitting, errors, isSubmitSuccessful } } = form

  async function onSubmit(values: FormValues) {
    try {
      await authApi.resetPassword({ token, password: values.password })
      // isSubmitSuccessful = true shows success message
    } catch (err: unknown) {
      setError("root", { message: extractApiError(err) })
    }
  }

  if (!token) {
    return (
      <div className="rounded-xl border border-border bg-card p-8 shadow-sm text-center space-y-4">
        <p className="text-sm text-destructive">Invalid or missing reset token.</p>
        <Link href="/forgot-password" className="text-sm text-primary hover:underline">
          Request a new link
        </Link>
      </div>
    )
  }

  if (isSubmitSuccessful) {
    return (
      <div className="rounded-xl border border-border bg-card p-8 shadow-sm text-center space-y-4">
        <h1 className="text-xl font-semibold">Password updated</h1>
        <p className="text-sm text-muted-foreground">
          Your password has been reset. All existing sessions have been revoked.
        </p>
        <Button className="w-full" onClick={() => router.push("/login")}>
          Sign in
        </Button>
      </div>
    )
  }

  return (
    <div className="rounded-xl border border-border bg-card p-8 shadow-sm space-y-6">
      <div className="space-y-1">
        <h1 className="text-xl font-semibold">Reset password</h1>
        <p className="text-sm text-muted-foreground">Enter your new password below.</p>
      </div>

      <form onSubmit={handleSubmit(onSubmit)} noValidate className="space-y-4">
        <FormField
          control={form.control}
          name="password"
          label="New password"
          type="password"
          autoComplete="new-password"
          description="Minimum 8 characters"
        />
        <FormField
          control={form.control}
          name="confirm"
          label="Confirm password"
          type="password"
          autoComplete="new-password"
        />

        {errors.root && (
          <p role="alert" className="text-sm text-destructive">{errors.root.message}</p>
        )}

        <Button type="submit" className="w-full" disabled={isSubmitting}>
          {isSubmitting ? "Resetting…" : "Reset password"}
        </Button>
      </form>
    </div>
  )
}

export default function ResetPasswordPage() {
  return (
    <Suspense>
      <ResetPasswordContent />
    </Suspense>
  )
}
