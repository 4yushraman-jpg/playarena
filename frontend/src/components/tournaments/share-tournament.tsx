"use client"

import { useState } from "react"
import { Share2Icon, CopyIcon, CheckIcon, GlobeIcon, LockIcon, LinkIcon } from "lucide-react"
import { toast } from "sonner"
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card"
import { Button } from "@/components/ui/button"
import { cn } from "@/lib/utils"
import { useUpdateTournament } from "@/hooks/use-tournaments"
import type { Tournament, TournamentVisibility } from "@/types/api/tournaments"

const OPTIONS: { value: TournamentVisibility; label: string; desc: string; icon: React.ReactNode }[] = [
  { value: "private", label: "Private", desc: "Only your organization. No public link.", icon: <LockIcon className="size-4" /> },
  { value: "unlisted", label: "Unlisted", desc: "Anyone with the link can view. Not listed or indexed.", icon: <LinkIcon className="size-4" /> },
  { value: "public", label: "Public", desc: "Anyone can view. Discoverable and indexed.", icon: <GlobeIcon className="size-4" /> },
]

interface ShareTournamentProps {
  orgSlug: string
  tournament: Tournament
}

/**
 * Organizer Share control (PRI-1): copy the public link and set visibility.
 * Setting visibility goes through the existing tournament.update permission;
 * draft tournaments are never public regardless of this setting.
 */
export function ShareTournament({ orgSlug, tournament }: ShareTournamentProps) {
  const [copied, setCopied] = useState(false)
  const update = useUpdateTournament(orgSlug, tournament.id)

  const publicUrl =
    typeof window !== "undefined"
      ? `${window.location.origin}/t/${orgSlug}/${tournament.slug}`
      : `/t/${orgSlug}/${tournament.slug}`

  const isShareable = tournament.visibility !== "private" && tournament.status !== "draft"

  async function copy() {
    try {
      await navigator.clipboard.writeText(publicUrl)
      setCopied(true)
      toast.success("Link copied")
      setTimeout(() => setCopied(false), 2000)
    } catch {
      toast.error("Could not copy link")
    }
  }

  function setVisibility(v: TournamentVisibility) {
    if (v === tournament.visibility) return
    update.mutate({ visibility: v }, { onSuccess: () => toast.success(`Visibility set to ${v}`) })
  }

  return (
    <Card>
      <CardHeader className="pb-3">
        <CardTitle className="flex items-center gap-2 text-base">
          <Share2Icon className="size-4 text-muted-foreground" />
          Share tournament
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        {tournament.status === "draft" && (
          <p className="text-sm text-muted-foreground">
            Draft tournaments are never public. Open registration (or beyond) and set a
            public visibility to share.
          </p>
        )}

        <div className="space-y-2">
          <span className="text-xs font-medium text-muted-foreground">Visibility</span>
          <div className="space-y-2">
            {OPTIONS.map((o) => (
              <button
                key={o.value}
                type="button"
                onClick={() => setVisibility(o.value)}
                aria-pressed={tournament.visibility === o.value}
                disabled={update.isPending}
                className={cn(
                  "flex w-full items-start gap-2.5 rounded-md border px-3 py-2 text-left transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:opacity-60",
                  tournament.visibility === o.value ? "border-primary bg-primary/5" : "border-input hover:bg-accent",
                )}
              >
                <span className="mt-0.5 text-muted-foreground">{o.icon}</span>
                <span>
                  <span className="block text-sm font-medium">{o.label}</span>
                  <span className="block text-xs text-muted-foreground">{o.desc}</span>
                </span>
              </button>
            ))}
          </div>
        </div>

        {isShareable && (
          <div className="space-y-1.5">
            <span className="text-xs font-medium text-muted-foreground">Public link</span>
            <div className="flex items-center gap-2">
              <code className="min-w-0 flex-1 truncate rounded-md border border-border bg-muted/40 px-2.5 py-1.5 text-xs">
                {publicUrl}
              </code>
              <Button variant="outline" size="sm" className="gap-1.5" onClick={copy}>
                {copied ? <CheckIcon className="size-3.5" /> : <CopyIcon className="size-3.5" />}
                {copied ? "Copied" : "Copy"}
              </Button>
            </div>
          </div>
        )}
      </CardContent>
    </Card>
  )
}
