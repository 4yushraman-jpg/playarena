import { format, formatDistanceToNow, parseISO, isValid } from "date-fns"

export function formatDate(iso: string | null | undefined, pattern = "MMM d, yyyy"): string {
  if (!iso) return "—"
  const d = parseISO(iso)
  return isValid(d) ? format(d, pattern) : "—"
}

export function formatDateTime(iso: string | null | undefined): string {
  return formatDate(iso, "MMM d, yyyy 'at' h:mm a")
}

export function formatRelative(iso: string | null | undefined): string {
  if (!iso) return "—"
  const d = parseISO(iso)
  return isValid(d) ? formatDistanceToNow(d, { addSuffix: true }) : "—"
}

export function formatScore(home: number, away: number): string {
  return `${home} – ${away}`
}

export function formatPrizePool(value: string | null | undefined): string {
  if (!value) return "—"
  const n = parseFloat(value)
  if (isNaN(n)) return value
  return new Intl.NumberFormat("en-IN", { style: "currency", currency: "INR", maximumFractionDigits: 0 }).format(n)
}

export function formatWinRate(rate: number): string {
  return `${(rate * 100).toFixed(1)}%`
}

export function formatOrdinal(n: number): string {
  const s = ["th", "st", "nd", "rd"]
  const v = n % 100
  return n + (s[(v - 20) % 10] ?? s[v] ?? s[0])
}
