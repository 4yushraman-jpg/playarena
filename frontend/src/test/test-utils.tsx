import React from "react"
import { render, type RenderOptions } from "@testing-library/react"
import { renderHook, type RenderHookOptions } from "@testing-library/react"
import { QueryClient, QueryClientProvider } from "@tanstack/react-query"

export function makeTestQueryClient() {
  return new QueryClient({
    defaultOptions: {
      queries: { retry: false, gcTime: 0 },
      mutations: { retry: false },
    },
  })
}

function TestProviders({ children, client }: { children: React.ReactNode; client: QueryClient }) {
  return <QueryClientProvider client={client}>{children}</QueryClientProvider>
}

export function renderWithProviders(
  ui: React.ReactElement,
  options?: RenderOptions & { client?: QueryClient },
) {
  const client = options?.client ?? makeTestQueryClient()
  return {
    client,
    ...render(ui, {
      wrapper: ({ children }) => <TestProviders client={client}>{children}</TestProviders>,
      ...options,
    }),
  }
}

export function renderHookWithProviders<T>(
  hook: () => T,
  options?: RenderHookOptions<T> & { client?: QueryClient },
) {
  const client = options?.client ?? makeTestQueryClient()
  return {
    client,
    ...renderHook(hook, {
      wrapper: ({ children }) => <TestProviders client={client}>{children as React.ReactNode}</TestProviders>,
      ...options,
    }),
  }
}
