import type { Metadata } from "next"
import Link from "next/link"
import { notFound } from "next/navigation"
import { TrophyIcon } from "lucide-react"
import { StatusBadge } from "@/components/ui/status-badge"
import { PublicTabs } from "@/components/public/public-tabs"
import { publicApi, PublicNotFoundError } from "@/lib/api/public"

interface RouteParams {
  params: Promise<{ orgSlug: string; tournamentSlug: string }>
}

// generateMetadata server-renders Open Graph / Twitter tags so a shared link
// unfurls into a rich card (WhatsApp/Instagram/Facebook/X) — a growth mechanism.
// Unlisted tournaments still get OG tags (for link previews) but are noindex'd.
export async function generateMetadata({ params }: RouteParams): Promise<Metadata> {
  const { orgSlug, tournamentSlug } = await params
  try {
    const t = await publicApi.tournament(orgSlug, tournamentSlug)
    const title = `${t.name} — ${t.organization.name}`
    const description =
      t.description ?? `${formatLabel(t.format)} ${t.sport} tournament on PlayArena.`
    const noindex = t.visibility !== "public"
    return {
      title,
      description,
      robots: noindex ? { index: false, follow: false } : undefined,
      openGraph: {
        title,
        description,
        type: "website",
        images: t.banner_url ? [{ url: t.banner_url }] : undefined,
      },
      twitter: {
        card: t.banner_url ? "summary_large_image" : "summary",
        title,
        description,
      },
    }
  } catch {
    return { title: "Tournament — PlayArena" }
  }
}

export default async function PublicTournamentLayout({
  params,
  children,
}: RouteParams & { children: React.ReactNode }) {
  const { orgSlug, tournamentSlug } = await params

  let tournament
  try {
    tournament = await publicApi.tournament(orgSlug, tournamentSlug)
  } catch (err) {
    if (err instanceof PublicNotFoundError) notFound()
    throw err
  }

  const basePath = `/t/${orgSlug}/${tournamentSlug}`

  return (
    <div className="mx-auto min-h-screen w-full max-w-3xl px-4 py-6">
      <header className="space-y-3">
        <div className="flex items-center gap-2 text-xs text-muted-foreground">
          <TrophyIcon className="size-3.5" aria-hidden="true" />
          <span>{tournament.organization.name}</span>
        </div>
        <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
          <h1 className="text-xl font-bold sm:text-2xl">{tournament.name}</h1>
          <StatusBadge status={tournament.status as never} />
        </div>
        <div className="border-b border-border">
          <PublicTabs basePath={basePath} format={tournament.format} />
        </div>
      </header>

      <main className="py-5">{children}</main>

      <footer className="mt-8 border-t border-border pt-4 text-center text-xs text-muted-foreground">
        Powered by{" "}
        <Link href="/" className="font-medium text-foreground hover:underline">
          PlayArena
        </Link>
      </footer>
    </div>
  )
}

function formatLabel(f: string): string {
  return f.replace(/_/g, " ")
}
