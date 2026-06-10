"use client"

import { useRouter } from "next/navigation"
import { ChevronDownIcon, BuildingIcon } from "lucide-react"
import { useQuery } from "@tanstack/react-query"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { Button } from "@/components/ui/button"
import { Skeleton } from "@/components/ui/skeleton"
import { orgKeys } from "@/lib/query-keys"
import { orgsApi } from "@/lib/api/organizations"
import { useAuthStore } from "@/stores/auth.store"
import { tokenManager } from "@/lib/api/client"
import { authApi } from "@/lib/api/auth"
import { getQueryClient } from "@/lib/api/query-client"
import { cn } from "@/lib/utils"

interface OrgSwitcherProps {
  currentOrgSlug: string
  className?: string
}

export function OrgSwitcher({ currentOrgSlug, className }: OrgSwitcherProps) {
  const router = useRouter()
  const { clearSession } = useAuthStore()

  const { data, isLoading } = useQuery({
    queryKey: orgKeys.list(),
    queryFn: () => orgsApi.list({ limit: 20 }).then((r) => r.data),
    staleTime: 5 * 60_000,
  })

  const orgs = data?.organizations ?? []
  const currentOrg = orgs.find((o) => o.slug === currentOrgSlug)
  const otherOrgs = orgs.filter((o) => o.slug !== currentOrgSlug)

  async function handleSwitchOrg() {
    // JWT is org-scoped; switching requires re-authentication.
    // Perform a graceful logout then send the user to /login.
    const refreshToken = tokenManager.getRefreshToken()
    if (refreshToken) {
      await authApi.logout({ refresh_token: refreshToken }).catch(() => {})
    }
    getQueryClient().clear()
    clearSession()
    router.push("/login")
  }

  if (isLoading) {
    return <Skeleton className="h-7 w-36" />
  }

  const displayName = currentOrg?.name ?? currentOrgSlug

  if (otherOrgs.length === 0) {
    return (
      <div className={cn("flex items-center gap-1.5 px-2", className)}>
        <BuildingIcon className="size-3.5 shrink-0 text-muted-foreground" />
        <span className="max-w-40 truncate text-sm font-medium">{displayName}</span>
      </div>
    )
  }

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button
          variant="ghost"
          size="sm"
          className={cn("max-w-48 gap-1.5", className)}
          aria-label={`Current organization: ${displayName}. Click to switch.`}
        >
          <BuildingIcon className="size-3.5 shrink-0 text-muted-foreground" />
          <span className="max-w-32 truncate text-sm font-medium">{displayName}</span>
          <ChevronDownIcon className="size-3.5 shrink-0 text-muted-foreground" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="start" className="w-56">
        <DropdownMenuLabel className="text-xs font-normal text-muted-foreground">
          Switch organization
        </DropdownMenuLabel>
        <DropdownMenuSeparator />
        <DropdownMenuItem className="gap-2 opacity-60 cursor-default focus:bg-transparent" disabled>
          <OrgInitial name={displayName} />
          <span className="flex-1 truncate">{displayName}</span>
          <span className="text-xs text-muted-foreground">Current</span>
        </DropdownMenuItem>
        {otherOrgs.map((org) => (
          <DropdownMenuItem
            key={org.id}
            onClick={() => handleSwitchOrg()}
            className="gap-2 cursor-pointer"
          >
            <OrgInitial name={org.name} />
            <span className="flex-1 truncate">{org.name}</span>
          </DropdownMenuItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

function OrgInitial({ name }: { name: string }) {
  return (
    <div className="flex size-5 shrink-0 items-center justify-center rounded bg-primary/10 text-primary text-[10px] font-bold select-none">
      {name.charAt(0).toUpperCase()}
    </div>
  )
}
