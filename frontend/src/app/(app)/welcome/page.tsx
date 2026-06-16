"use client"

import { useEffect } from "react"
import Link from "next/link"
import { Building2Icon, TrophyIcon, UserIcon, ArrowRightIcon } from "lucide-react"
import { useAuthStore } from "@/stores/auth.store"

/**
 * Neutral post-login landing (PRI-1). A brand-new user is no longer force-marched
 * into organization creation — they choose. Organizer tooling is available now;
 * the player persona is shown as a forward-looking, disabled option (GP-2 is not
 * activated). This page only changes routing/UX — the onboarding token, scope, and
 * organization.create gating are unchanged.
 */
export default function WelcomePage() {
  const { claims, hydrateClaims } = useAuthStore()
  useEffect(() => {
    if (!claims) hydrateClaims()
  }, [claims, hydrateClaims])

  return (
    <div className="mx-auto w-full max-w-2xl px-4 py-12">
      <div className="space-y-2">
        <h1 className="text-2xl font-bold">Welcome to PlayArena</h1>
        <p className="text-sm text-muted-foreground">
          What would you like to do? You can change this anytime.
        </p>
      </div>

      <div className="mt-8 space-y-3">
        <Link
          href="/onboarding"
          className="group flex items-center gap-4 rounded-xl border border-border p-4 transition-colors hover:border-primary hover:bg-accent/40 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
        >
          <Building2Icon className="size-6 shrink-0 text-primary" aria-hidden="true" />
          <div className="min-w-0 flex-1">
            <p className="font-medium">Create an organization</p>
            <p className="text-sm text-muted-foreground">
              Run tournaments: teams, registrations, fixtures, live scoring and standings.
            </p>
          </div>
          <ArrowRightIcon className="size-4 shrink-0 text-muted-foreground transition-transform group-hover:translate-x-0.5" />
        </Link>

        <div className="flex items-start gap-4 rounded-xl border border-border p-4">
          <TrophyIcon className="size-6 shrink-0 text-muted-foreground" aria-hidden="true" />
          <div className="min-w-0 flex-1">
            <p className="font-medium">Browse tournaments</p>
            <p className="text-sm text-muted-foreground">
              Have a tournament link? Open it to follow fixtures, brackets, standings and
              results — no account needed to view.
            </p>
          </div>
        </div>

        <div className="flex items-start gap-4 rounded-xl border border-dashed border-border p-4 opacity-70">
          <UserIcon className="size-6 shrink-0 text-muted-foreground" aria-hidden="true" />
          <div className="min-w-0 flex-1">
            <p className="flex items-center gap-2 font-medium">
              Player profiles
              <span className="rounded-full bg-muted px-2 py-0.5 text-[10px] font-medium uppercase tracking-wide text-muted-foreground">
                Coming soon
              </span>
            </p>
            <p className="text-sm text-muted-foreground">
              Your own player identity, history and ranking — on the way.
            </p>
          </div>
        </div>
      </div>
    </div>
  )
}
