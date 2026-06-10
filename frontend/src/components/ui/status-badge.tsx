import { cn } from "@/lib/utils"

// --- Status value unions (must match backend enums) ---

export type MatchStatus = "scheduled" | "live" | "completed" | "cancelled" | "abandoned"
export type TournamentStatus = "draft" | "registration_open" | "registration_closed" | "ongoing" | "completed" | "cancelled"
export type RegistrationStatus = "pending" | "approved" | "rejected" | "withdrawn" | "disqualified"
export type PlayerStatus = "active" | "inactive"
export type TeamStatus = "active" | "inactive"

type StatusType =
  | MatchStatus
  | TournamentStatus
  | RegistrationStatus
  | PlayerStatus
  | TeamStatus

// --- Label + color mappings ---

const STATUS_CONFIG: Record<StatusType, { label: string; className: string }> = {
  // Match
  scheduled:           { label: "Scheduled",           className: "bg-blue-50   text-blue-700   border-blue-200   dark:bg-blue-950/40  dark:text-blue-300   dark:border-blue-800" },
  live:                { label: "Live",                 className: "bg-green-50  text-green-700  border-green-200  dark:bg-green-950/40 dark:text-green-300  dark:border-green-800 animate-pulse" },
  completed:           { label: "Completed",            className: "bg-neutral-100 text-neutral-600 border-neutral-200 dark:bg-neutral-800 dark:text-neutral-400 dark:border-neutral-700" },
  cancelled:           { label: "Cancelled",            className: "bg-red-50    text-red-700    border-red-200    dark:bg-red-950/40   dark:text-red-300    dark:border-red-800" },
  abandoned:           { label: "Abandoned",            className: "bg-orange-50 text-orange-700 border-orange-200 dark:bg-orange-950/40 dark:text-orange-300 dark:border-orange-800" },
  // Tournament
  draft:               { label: "Draft",                className: "bg-neutral-100 text-neutral-600 border-neutral-200 dark:bg-neutral-800 dark:text-neutral-400 dark:border-neutral-700" },
  registration_open:   { label: "Registration Open",    className: "bg-green-50  text-green-700  border-green-200  dark:bg-green-950/40 dark:text-green-300  dark:border-green-800" },
  registration_closed: { label: "Reg. Closed",          className: "bg-blue-50   text-blue-700   border-blue-200   dark:bg-blue-950/40  dark:text-blue-300   dark:border-blue-800" },
  ongoing:             { label: "Ongoing",              className: "bg-amber-50  text-amber-700  border-amber-200  dark:bg-amber-950/40 dark:text-amber-300  dark:border-amber-800" },
  // Registration
  pending:             { label: "Pending",              className: "bg-amber-50  text-amber-700  border-amber-200  dark:bg-amber-950/40 dark:text-amber-300  dark:border-amber-800" },
  approved:            { label: "Approved",             className: "bg-green-50  text-green-700  border-green-200  dark:bg-green-950/40 dark:text-green-300  dark:border-green-800" },
  rejected:            { label: "Rejected",             className: "bg-red-50    text-red-700    border-red-200    dark:bg-red-950/40   dark:text-red-300    dark:border-red-800" },
  withdrawn:           { label: "Withdrawn",            className: "bg-neutral-100 text-neutral-600 border-neutral-200 dark:bg-neutral-800 dark:text-neutral-400 dark:border-neutral-700" },
  disqualified:        { label: "Disqualified",         className: "bg-red-50    text-red-700    border-red-200    dark:bg-red-950/40   dark:text-red-300    dark:border-red-800" },
  // Player/Team
  active:              { label: "Active",               className: "bg-green-50  text-green-700  border-green-200  dark:bg-green-950/40 dark:text-green-300  dark:border-green-800" },
  inactive:            { label: "Inactive",             className: "bg-neutral-100 text-neutral-600 border-neutral-200 dark:bg-neutral-800 dark:text-neutral-400 dark:border-neutral-700" },
}

interface StatusBadgeProps {
  status: StatusType
  className?: string
}

export function StatusBadge({ status, className }: StatusBadgeProps) {
  const config = STATUS_CONFIG[status]
  if (!config) return null

  return (
    <span
      className={cn(
        "inline-flex h-5 items-center rounded-full border px-2 py-0.5 text-xs font-medium",
        config.className,
        className,
      )}
    >
      {config.label}
    </span>
  )
}
