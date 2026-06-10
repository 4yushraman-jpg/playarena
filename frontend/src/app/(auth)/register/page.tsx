"use client"

import Link from "next/link"
import { useRouter } from "next/navigation"
import { useForm } from "react-hook-form"
import { zodResolver } from "@hookform/resolvers/zod"
import { z } from "zod"
import { Button } from "@/components/ui/button"
import { FormField } from "@/components/ui/form-field"
import { authApi } from "@/lib/api/auth"
import { extractApiError, extractFieldErrors } from "@/lib/api-error"

const schema = z.object({
  full_name: z.string().min(1, "Full name is required"),
  username: z
    .string()
    .min(3, "Username must be at least 3 characters")
    .max(30, "Username must be at most 30 characters")
    .regex(/^[a-zA-Z0-9_]+$/, "Only letters, numbers, and underscores"),
  email: z.string().email("Enter a valid email"),
  password: z
    .string()
    .min(8, "Password must be at least 8 characters")
    .max(72, "Password must be at most 72 characters"),
})

type FormValues = z.infer<typeof schema>

export default function RegisterPage() {
  const router = useRouter()

  const form = useForm<FormValues>({
    resolver: zodResolver(schema),
    defaultValues: { full_name: "", username: "", email: "", password: "" },
  })

  const { handleSubmit, setError, formState: { isSubmitting, errors } } = form

  async function onSubmit(values: FormValues) {
    try {
      await authApi.register(values)
      router.push(`/verify-email?email=${encodeURIComponent(values.email)}`)
    } catch (err: unknown) {
      const fields = extractFieldErrors(err)
      if (Object.keys(fields).length > 0) {
        for (const [field, msg] of Object.entries(fields)) {
          setError(field as keyof FormValues, { message: msg })
        }
        return
      }
      setError("root", { message: extractApiError(err) })
    }
  }

  return (
    <div className="rounded-xl border border-border bg-card p-8 shadow-sm space-y-6">
      <div className="space-y-1">
        <h1 className="text-xl font-semibold">Create an account</h1>
        <p className="text-sm text-muted-foreground">Start managing your tournaments</p>
      </div>

      <form onSubmit={handleSubmit(onSubmit)} noValidate className="space-y-4">
        <FormField control={form.control} name="full_name" label="Full name" autoComplete="name" />
        <FormField control={form.control} name="username" label="Username" autoComplete="username" />
        <FormField control={form.control} name="email" label="Email" type="email" autoComplete="email" />
        <FormField
          control={form.control}
          name="password"
          label="Password"
          type="password"
          autoComplete="new-password"
          description="Minimum 8 characters"
        />

        {errors.root && (
          <p role="alert" className="text-sm text-destructive">{errors.root.message}</p>
        )}

        <Button type="submit" className="w-full" disabled={isSubmitting}>
          {isSubmitting ? "Creating account…" : "Create account"}
        </Button>
      </form>

      <p className="text-center text-sm text-muted-foreground">
        Already have an account?{" "}
        <Link href="/login" className="text-primary hover:underline">
          Sign in
        </Link>
      </p>
    </div>
  )
}
