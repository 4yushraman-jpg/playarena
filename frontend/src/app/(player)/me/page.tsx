"use client"

import { useAuthStore, selectScope, selectIsAuthenticated } from "@/stores/auth.store"

// GP-1 foundation placeholder for the player home. This is intentionally
// minimal — the full player dashboard is a later phase. It exists so the
// player persona has a route to land on and so the (player) route group and
// scope-aware guard are exercised end to end.
export default function PlayerHomePage() {
  const isAuthenticated = useAuthStore(selectIsAuthenticated)
  const scope = useAuthStore(selectScope)

  return (
    <main className="mx-auto max-w-2xl p-8">
      <h1 className="text-2xl font-semibold">Player</h1>
      <p className="mt-2 text-sm text-muted-foreground">
        {isAuthenticated
          ? `Signed in as a player (scope: ${scope ?? "unknown"}).`
          : "Not signed in."}
      </p>
      <p className="mt-4 text-sm text-muted-foreground">
        Your global player profile and reputation will appear here. (GP-1 foundation)
      </p>
    </main>
  )
}
