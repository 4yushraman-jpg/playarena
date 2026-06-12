"use client"

import { useEffect } from "react"
import { useRouter } from "next/navigation"
import { useForm } from "react-hook-form"
import { zodResolver } from "@hookform/resolvers/zod"
import { z } from "zod"
import { Button } from "@/components/ui/button"
import { FormField, FormSelect } from "@/components/ui/form-field"
import { authApi } from "@/lib/api/auth"
import { orgsApi } from "@/lib/api/organizations"
import { tokenManager } from "@/lib/api/client"
import { extractApiError } from "@/lib/api-error"
import { useAuthStore } from "@/stores/auth.store"
import type { OrganizationType } from "@/types/api/organizations"

const schema = z.object({
  name: z.string().trim().min(3, "Organization name must be at least 3 characters"),
  type: z.enum(["club", "federation", "school", "corporate", "independent"]),
  city: z.string().trim().optional(),
  country: z.string().trim().length(2, "Use a 2-letter country code").optional().or(z.literal("")),
})

type FormValues = z.infer<typeof schema>

const orgTypeOptions = [
  { value: "club", label: "Club" },
  { value: "federation", label: "Federation" },
  { value: "school", label: "School" },
  { value: "corporate", label: "Corporate" },
  { value: "independent", label: "Independent" },
]

export default function OnboardingPage() {
  const router = useRouter()
  const { claims, hydrateClaims, setSession, setOrgSlug } = useAuthStore()

  useEffect(() => {
    if (!claims) hydrateClaims()
  }, [claims, hydrateClaims])

  const form = useForm<FormValues>({
    resolver: zodResolver(schema),
    defaultValues: { name: "", type: "club", city: "", country: "" },
  })

  const { handleSubmit, setError, formState: { isSubmitting, errors } } = form

  async function onSubmit(values: FormValues) {
    try {
      const { data: org } = await orgsApi.create({
        name: values.name,
        type: values.type as OrganizationType,
        ...(values.city ? { city: values.city } : {}),
        ...(values.country ? { country: values.country.toUpperCase() } : {}),
      })

      const refreshToken = tokenManager.getRefreshToken()
      if (!refreshToken) {
        router.push("/login")
        return
      }

      const { data: tokens } = await authApi.refresh(refreshToken, org.id)
      setSession(tokens)
      setOrgSlug(org.slug)
      router.push(`/${org.slug}`)
    } catch (err: unknown) {
      setError("root", { message: extractApiError(err) })
    }
  }

  return (
    <div className="rounded-xl border border-border bg-card p-8 shadow-sm space-y-6">
      <div className="space-y-1">
        <h1 className="text-xl font-semibold">Create your organization</h1>
        <p className="text-sm text-muted-foreground">Set up the workspace your tournaments will belong to.</p>
      </div>

      <form onSubmit={handleSubmit(onSubmit)} noValidate className="space-y-4">
        <FormField
          control={form.control}
          name="name"
          label="Organization name"
          autoComplete="organization"
          placeholder="Downtown Sports Club"
          required
        />
        <FormSelect
          control={form.control}
          name="type"
          label="Organization type"
          options={orgTypeOptions}
          required
        />
        <div className="grid gap-4 sm:grid-cols-2">
          <FormField control={form.control} name="city" label="City" placeholder="Mumbai" />
          <FormField control={form.control} name="country" label="Country" placeholder="IN" maxLength={2} />
        </div>

        {errors.root && (
          <p role="alert" className="text-sm text-destructive">{errors.root.message}</p>
        )}

        <Button type="submit" className="w-full" disabled={isSubmitting}>
          {isSubmitting ? "Creating organization..." : "Continue"}
        </Button>
      </form>
    </div>
  )
}
