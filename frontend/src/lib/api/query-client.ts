import { QueryClient } from "@tanstack/react-query"
import { extractApiError } from "@/lib/api-error"

export function makeQueryClient() {
  return new QueryClient({
    defaultOptions: {
      queries: {
        // 30 seconds — org data is stable enough to avoid redundant fetches
        // on tab focus, but fresh enough for real-time admin workflows.
        staleTime: 30 * 1000,
        // Keep unused data in cache for 5 minutes before GC.
        gcTime: 5 * 60 * 1000,
        // One retry on failure; don't hammer a flaky backend.
        retry: 1,
        refetchOnWindowFocus: true,
        refetchOnReconnect: true,
      },
      mutations: {
        onError: (error) => {
          // Global fallback; individual mutations can override with their own
          // onError to map field-level errors to form state.
          const message = extractApiError(error)
          // Toast is wired via the Toaster in the root layout.
          // We import dynamically to avoid a circular dep between query-client
          // (infrastructure) and sonner (UI).
          import("sonner").then(({ toast }) => toast.error(message))
        },
      },
    },
  })
}

let browserClient: QueryClient | undefined

export function getQueryClient() {
  if (typeof window === "undefined") {
    // Server: always make a new client to avoid sharing state between requests.
    return makeQueryClient()
  }
  // Browser: reuse a single instance.
  if (!browserClient) browserClient = makeQueryClient()
  return browserClient
}
