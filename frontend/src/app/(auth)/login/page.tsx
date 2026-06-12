"use client"

import Link from "next/link"
import { useRouter } from "next/navigation"
import { useForm } from "react-hook-form"
import { zodResolver } from "@hookform/resolvers/zod"
import { z } from "zod"
import { Button } from "@/components/ui/button"
import { FormField } from "@/components/ui/form-field"
import { authApi } from "@/lib/api/auth"
import { orgsApi } from "@/lib/api/organizations"
import { useAuthStore } from "@/stores/auth.store"
import { isOrgRequiredError, extractApiError } from "@/lib/api-error"
import type { OrgRequiredResponse } from "@/types/api/auth"
import type { AxiosError } from "axios"

const schema = z.object({
  email: z.string().email("Enter a valid email"),
  password: z.string().min(8, "Password must be at least 8 characters"),
})

type FormValues = z.infer<typeof schema>

export default function LoginPage() {
  const router = useRouter()
  const { setSession, setOrgSlug, setPendingOrgSelection } = useAuthStore()

  const form = useForm<FormValues>({
    resolver: zodResolver(schema),
    defaultValues: { email: "", password: "" },
  })

  const { handleSubmit, setError, formState: { isSubmitting, errors } } = form

  async function onSubmit(values: FormValues) {
    try {
      const { data } = await authApi.login(values)
      setSession(data)
      const claims = useAuthStore.getState().claims
      if (!claims?.organizationId) {
        router.push(claims?.role === "onboarding" ? "/onboarding" : "/")
        return
      }

      // Resolve the slug for the org id embedded in the token.
      const { data: orgs } = await orgsApi.list({ limit: 200 })
      const slug = orgs.organizations.find((org) => org.id === claims.organizationId)?.slug
      if (slug) {
        setOrgSlug(slug)
        router.push(`/${slug}`)
      } else {
        router.push("/")
      }
    } catch (err: unknown) {
      if (isOrgRequiredError(err)) {
        const axiosErr = err as AxiosError<OrgRequiredResponse>
        const orgs = axiosErr.response?.data?.organizations ?? []
        // Store credentials temporarily so org-select can re-login with organization_id.
        // Both are cleared immediately after use in org-select/page.tsx.
        sessionStorage.setItem("pa_pending_email", values.email)
        sessionStorage.setItem("pa_pending_password", values.password)
        setPendingOrgSelection(orgs)
        router.push("/org-select")
        return
      }
      setError("root", { message: extractApiError(err) })
    }
  }

  return (
    <div className="rounded-xl border border-border bg-card p-8 shadow-sm space-y-6">
      <div className="space-y-1">
        <h1 className="text-xl font-semibold">Sign in</h1>
        <p className="text-sm text-muted-foreground">Welcome back to PlayArena</p>
      </div>

      <form onSubmit={handleSubmit(onSubmit)} noValidate className="space-y-4">
        <FormField
          control={form.control}
          name="email"
          label="Email"
          type="email"
          autoComplete="email"
          placeholder="you@example.com"
        />
        <FormField
          control={form.control}
          name="password"
          label="Password"
          type="password"
          autoComplete="current-password"
          placeholder="••••••••"
        />

        {errors.root && (
          <p role="alert" className="text-sm text-destructive">{errors.root.message}</p>
        )}

        <Button type="submit" className="w-full" disabled={isSubmitting}>
          {isSubmitting ? "Signing in…" : "Sign in"}
        </Button>
      </form>

      <div className="space-y-2 text-center text-sm text-muted-foreground">
        <p>
          <Link href="/forgot-password" className="text-primary hover:underline">
            Forgot password?
          </Link>
        </p>
        <p>
          Don&apos;t have an account?{" "}
          <Link href="/register" className="text-primary hover:underline">
            Create one
          </Link>
        </p>
      </div>
    </div>
  )
}
