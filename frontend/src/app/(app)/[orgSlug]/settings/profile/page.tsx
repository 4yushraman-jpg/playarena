"use client"

import { useEffect } from "react"
import { useForm } from "react-hook-form"
import { zodResolver } from "@hookform/resolvers/zod"
import { z } from "zod"
import { useMutation, useQueryClient } from "@tanstack/react-query"
import { Loader2Icon, UserCircle2Icon } from "lucide-react"
import { toast } from "sonner"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { FormField } from "@/components/ui/form-field"
import { Separator } from "@/components/ui/separator"
import { useCurrentUser } from "@/hooks/use-current-user"
import { usersApi } from "@/lib/api/users"
import { userKeys } from "@/lib/query-keys"
import { extractApiError } from "@/lib/api-error"
import { useAuthStore, selectUserId } from "@/stores/auth.store"

const schema = z.object({
  full_name: z
    .string()
    .min(1, "Full name is required")
    .max(120, "Full name is too long"),
  username: z
    .string()
    .min(3, "Username must be at least 3 characters")
    .max(50, "Username is too long")
    .regex(/^[a-z0-9_-]+$/, "Only lowercase letters, numbers, underscores, and hyphens"),
})

type FormValues = z.infer<typeof schema>

export default function ProfileSettingsPage() {
  const userId = useAuthStore(selectUserId)
  const queryClient = useQueryClient()
  const { data: user, isLoading, isError } = useCurrentUser()

  const form = useForm<FormValues>({
    resolver: zodResolver(schema),
    defaultValues: { full_name: "", username: "" },
    mode: "onBlur",
  })

  // Populate form once user data loads
  useEffect(() => {
    if (user) {
      form.reset({ full_name: user.full_name, username: user.username })
    }
  }, [user, form])

  const mutation = useMutation({
    mutationFn: (data: FormValues) => usersApi.update(userId!, data),
    onSuccess: (response) => {
      queryClient.setQueryData(userKeys.detail(userId!), response.data)
      form.reset({ full_name: response.data.full_name, username: response.data.username })
      toast.success("Profile saved")
    },
    onError: (err) => {
      const msg = extractApiError(err)
      toast.error(msg ?? "Failed to save profile")
    },
  })

  const isDirty = form.formState.isDirty

  if (isError) {
    return (
      <div className="rounded-xl border border-dashed border-border p-8 text-center">
        <p className="text-sm text-muted-foreground">Failed to load profile. Please refresh.</p>
      </div>
    )
  }

  return (
    <div className="space-y-6">
      <div>
        <h2 className="text-lg font-semibold">Profile</h2>
        <p className="text-sm text-muted-foreground">
          Update your display name and username.
        </p>
      </div>

      <Separator />

      {/* Avatar placeholder */}
      <div className="flex items-center gap-4">
        <div
          className="flex size-16 items-center justify-center rounded-full bg-muted text-muted-foreground"
          aria-hidden="true"
        >
          {isLoading ? (
            <Skeleton className="size-16 rounded-full" />
          ) : (
            <UserCircle2Icon className="size-10 text-muted-foreground/50" />
          )}
        </div>
        <div>
          {isLoading ? (
            <div className="space-y-1.5">
              <Skeleton className="h-4 w-32" />
              <Skeleton className="h-3 w-48" />
            </div>
          ) : (
            <>
              <p className="text-sm font-medium">{user?.full_name || "—"}</p>
              <p className="text-xs text-muted-foreground">{user?.email}</p>
            </>
          )}
        </div>
      </div>

      <Separator />

      {isLoading ? (
        <ProfileFormSkeleton />
      ) : (
        <form
          onSubmit={form.handleSubmit((data) => mutation.mutate(data))}
          noValidate
          className="space-y-5"
        >
          <FormField
            control={form.control}
            name="full_name"
            label="Full name"
            placeholder="Your full name"
            required
            autoComplete="name"
          />

          <FormField
            control={form.control}
            name="username"
            label="Username"
            placeholder="your_username"
            required
            autoComplete="username"
            description="3–50 characters. Lowercase letters, numbers, underscores, and hyphens only."
          />

          {/* Read-only email */}
          <div className="space-y-1.5">
            <p className="text-sm font-medium text-foreground">Email</p>
            <div className="flex h-9 items-center rounded-lg border border-input bg-muted/40 px-3 text-sm text-muted-foreground">
              {user?.email}
            </div>
            <p className="text-xs text-muted-foreground">
              Email cannot be changed.
            </p>
          </div>

          <div className="flex items-center gap-3 pt-2">
            <Button
              type="submit"
              disabled={!isDirty || mutation.isPending}
              className="gap-2"
            >
              {mutation.isPending && <Loader2Icon className="size-3.5 animate-spin" />}
              Save changes
            </Button>
            {isDirty && (
              <Button
                type="button"
                variant="ghost"
                onClick={() =>
                  form.reset({ full_name: user?.full_name, username: user?.username })
                }
                disabled={mutation.isPending}
              >
                Cancel
              </Button>
            )}
            {isDirty && (
              <span className="text-xs text-muted-foreground">
                You have unsaved changes
              </span>
            )}
          </div>
        </form>
      )}
    </div>
  )
}

function ProfileFormSkeleton() {
  return (
    <div className="space-y-5" aria-busy="true" aria-label="Loading profile form">
      {[1, 2].map((i) => (
        <div key={i} className="space-y-1.5">
          <Skeleton className="h-4 w-20" />
          <Skeleton className="h-9 w-full" />
        </div>
      ))}
      <Skeleton className="h-9 w-28" />
    </div>
  )
}
