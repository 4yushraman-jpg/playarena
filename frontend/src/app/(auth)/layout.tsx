import type { Metadata } from "next"

export const metadata: Metadata = {
  title: { absolute: "PlayArena" },
}

export default function AuthLayout({ children }: { children: React.ReactNode }) {
  return (
    <div className="flex min-h-svh flex-col items-center justify-center bg-muted/30 px-4 py-12">
      <div className="w-full max-w-md space-y-6">
        <div className="flex flex-col items-center gap-2 text-center">
          <div className="flex size-10 items-center justify-center rounded-xl bg-primary text-primary-foreground font-bold text-lg select-none">
            PA
          </div>
          <span className="text-xl font-semibold tracking-tight">PlayArena</span>
        </div>
        {children}
      </div>
    </div>
  )
}
