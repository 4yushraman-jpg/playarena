import { Skeleton } from "@/components/ui/skeleton"
import { cn } from "@/lib/utils"

// ── Table skeleton ─────────────────────────────────────────────────────────────

interface TableSkeletonProps {
  rows?: number
  columns?: number
  className?: string
}

export function TableSkeleton({ rows = 6, columns = 4, className }: TableSkeletonProps) {
  return (
    <div className={cn("space-y-0 overflow-hidden rounded-lg border border-border", className)}>
      {/* Header */}
      <div className="flex gap-4 border-b bg-muted/40 px-4 py-3">
        {Array.from({ length: columns }).map((_, i) => (
          <Skeleton key={i} className="h-4 flex-1" />
        ))}
      </div>
      {/* Rows */}
      {Array.from({ length: rows }).map((_, i) => (
        <div key={i} className="flex gap-4 border-b px-4 py-3 last:border-0">
          {Array.from({ length: columns }).map((_, j) => (
            <Skeleton key={j} className="h-4 flex-1" style={{ opacity: 1 - i * 0.07 }} />
          ))}
        </div>
      ))}
    </div>
  )
}

// ── Card skeleton ──────────────────────────────────────────────────────────────

interface CardSkeletonProps {
  className?: string
}

export function CardSkeleton({ className }: CardSkeletonProps) {
  return (
    <div className={cn("rounded-lg border border-border bg-card p-5 space-y-3", className)}>
      <Skeleton className="h-5 w-2/5" />
      <Skeleton className="h-3 w-4/5" />
      <Skeleton className="h-3 w-3/5" />
      <div className="pt-2 flex gap-2">
        <Skeleton className="h-7 w-20 rounded-md" />
        <Skeleton className="h-7 w-16 rounded-md" />
      </div>
    </div>
  )
}

// ── Stat card skeleton ─────────────────────────────────────────────────────────

interface StatSkeletonProps {
  count?: number
  className?: string
}

export function StatSkeleton({ count = 4, className }: StatSkeletonProps) {
  return (
    <div className={cn("grid gap-4 sm:grid-cols-2 lg:grid-cols-4", className)}>
      {Array.from({ length: count }).map((_, i) => (
        <div key={i} className="rounded-lg border border-border bg-card p-5 space-y-2">
          <Skeleton className="h-3 w-24" />
          <Skeleton className="h-7 w-16" />
          <Skeleton className="h-3 w-32" />
        </div>
      ))}
    </div>
  )
}

// ── Page skeleton (title + table) ─────────────────────────────────────────────

export function PageSkeleton() {
  return (
    <div className="space-y-6">
      <div className="space-y-2">
        <Skeleton className="h-7 w-48" />
        <Skeleton className="h-4 w-80" />
      </div>
      <TableSkeleton rows={8} columns={5} />
    </div>
  )
}
