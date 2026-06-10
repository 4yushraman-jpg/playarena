"use client"

import { useState } from "react"
import { QueryClientProvider } from "@tanstack/react-query"
import { ThemeProvider } from "next-themes"
import { Toaster } from "sonner"
import { makeQueryClient } from "@/lib/api/query-client"

export function Providers({ children }: { children: React.ReactNode }) {
  // useState ensures the QueryClient is not recreated on every render.
  const [queryClient] = useState(() => makeQueryClient())

  return (
    <QueryClientProvider client={queryClient}>
      <ThemeProvider
        attribute="class"
        defaultTheme="system"
        enableSystem
        disableTransitionOnChange
      >
        {children}
        <Toaster richColors closeButton position="bottom-right" />
      </ThemeProvider>
    </QueryClientProvider>
  )
}
