"use client"

import * as React from "react"
import {
  type ColumnDef,
  type SortingState,
  type ColumnFiltersState,
  type PaginationState,
  flexRender,
  getCoreRowModel,
  getSortedRowModel,
  getPaginationRowModel,
  useReactTable,
} from "@tanstack/react-table"
import { ArrowUpIcon, ArrowDownIcon, ChevronsUpDownIcon, ChevronLeftIcon, ChevronRightIcon } from "lucide-react"
import { cn } from "@/lib/utils"
import { Button } from "@/components/ui/button"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { Skeleton } from "@/components/ui/skeleton"

// ── Types ─────────────────────────────────────────────────────────────────────

interface DataTableProps<TData, TValue> {
  columns: ColumnDef<TData, TValue>[]
  data: TData[]
  /** Total record count for server-side pagination */
  total?: number
  isLoading?: boolean
  /** Controlled pagination (server-side). Omit for client-side. */
  pagination?: PaginationState
  onPaginationChange?: React.Dispatch<React.SetStateAction<PaginationState>>
  /** Controlled sorting (server-side). Omit for client-side. */
  sorting?: SortingState
  onSortingChange?: React.Dispatch<React.SetStateAction<SortingState>>
  emptyMessage?: string
  className?: string
}

// ── Sort indicator ─────────────────────────────────────────────────────────────

function SortIcon({ direction }: { direction: false | "asc" | "desc" }) {
  if (direction === "asc") return <ArrowUpIcon className="size-3.5" />
  if (direction === "desc") return <ArrowDownIcon className="size-3.5" />
  return <ChevronsUpDownIcon className="size-3.5 opacity-40" />
}

// ── Loading rows ───────────────────────────────────────────────────────────────

function SkeletonRows({ columns, rows = 6 }: { columns: number; rows?: number }) {
  return (
    <>
      {Array.from({ length: rows }).map((_, i) => (
        <TableRow key={i}>
          {Array.from({ length: columns }).map((_, j) => (
            <TableCell key={j}>
              <Skeleton className="h-4 w-full" />
            </TableCell>
          ))}
        </TableRow>
      ))}
    </>
  )
}

// ── Component ─────────────────────────────────────────────────────────────────

export function DataTable<TData, TValue>({
  columns,
  data,
  total,
  isLoading = false,
  pagination: controlledPagination,
  onPaginationChange,
  sorting: controlledSorting,
  onSortingChange,
  emptyMessage = "No results.",
  className,
}: DataTableProps<TData, TValue>) {
  const [internalSorting, setInternalSorting] = React.useState<SortingState>([])
  const [internalPagination, setInternalPagination] = React.useState<PaginationState>({
    pageIndex: 0,
    pageSize: 20,
  })

  const isServerPaginated = Boolean(controlledPagination && onPaginationChange)
  const isServerSorted = Boolean(controlledSorting && onSortingChange)

  const pagination = controlledPagination ?? internalPagination
  const sorting = controlledSorting ?? internalSorting

  const pageCount = isServerPaginated && total != null
    ? Math.ceil(total / pagination.pageSize)
    : undefined

  // eslint-disable-next-line react-hooks/incompatible-library -- TanStack Table v8 returns mutable functions; known React Compiler incompatibility, non-actionable
  const table = useReactTable({
    data,
    columns,
    state: { sorting, pagination },
    pageCount,
    manualPagination: isServerPaginated,
    manualSorting: isServerSorted,
    onSortingChange: isServerSorted ? onSortingChange! : setInternalSorting,
    onPaginationChange: isServerPaginated ? onPaginationChange! : setInternalPagination,
    getCoreRowModel: getCoreRowModel(),
    getSortedRowModel: isServerSorted ? undefined : getSortedRowModel(),
    getPaginationRowModel: isServerPaginated ? undefined : getPaginationRowModel(),
  })

  const canPrev = table.getCanPreviousPage()
  const canNext = table.getCanNextPage()
  const { pageIndex, pageSize } = table.getState().pagination
  const displayTotal = isServerPaginated ? (total ?? 0) : table.getFilteredRowModel().rows.length
  const from = displayTotal === 0 ? 0 : pageIndex * pageSize + 1
  const to = Math.min((pageIndex + 1) * pageSize, displayTotal)

  return (
    <div className={cn("space-y-3", className)}>
      <div className="overflow-hidden rounded-lg border border-border">
        <Table>
          <TableHeader>
            {table.getHeaderGroups().map((headerGroup) => (
              <TableRow key={headerGroup.id} className="bg-muted/40 hover:bg-muted/40">
                {headerGroup.headers.map((header) => {
                  const canSort = header.column.getCanSort()
                  const sorted = header.column.getIsSorted()
                  return (
                    <TableHead
                      key={header.id}
                      className={cn(canSort && "cursor-pointer select-none")}
                      onClick={canSort ? header.column.getToggleSortingHandler() : undefined}
                    >
                      {header.isPlaceholder ? null : (
                        <div className="flex items-center gap-1.5">
                          {flexRender(header.column.columnDef.header, header.getContext())}
                          {canSort && <SortIcon direction={sorted} />}
                        </div>
                      )}
                    </TableHead>
                  )
                })}
              </TableRow>
            ))}
          </TableHeader>
          <TableBody>
            {isLoading ? (
              <SkeletonRows columns={columns.length} />
            ) : table.getRowModel().rows.length === 0 ? (
              <TableRow>
                <TableCell
                  colSpan={columns.length}
                  className="h-32 text-center text-sm text-muted-foreground"
                >
                  {emptyMessage}
                </TableCell>
              </TableRow>
            ) : (
              table.getRowModel().rows.map((row) => (
                <TableRow key={row.id} data-state={row.getIsSelected() && "selected"}>
                  {row.getVisibleCells().map((cell) => (
                    <TableCell key={cell.id}>
                      {flexRender(cell.column.columnDef.cell, cell.getContext())}
                    </TableCell>
                  ))}
                </TableRow>
              ))
            )}
          </TableBody>
        </Table>
      </div>

      {/* Pagination bar — only shown when there is more than one page or server-paginated */}
      {(isServerPaginated || displayTotal > pageSize) && (
        <div className="flex items-center justify-between gap-4 px-1 text-sm text-muted-foreground">
          <span>
            {displayTotal === 0
              ? "No results"
              : `${from}–${to} of ${displayTotal.toLocaleString()}`}
          </span>
          <div className="flex items-center gap-1">
            <Button
              variant="outline"
              size="icon-sm"
              onClick={() => table.previousPage()}
              disabled={!canPrev || isLoading}
              aria-label="Previous page"
            >
              <ChevronLeftIcon />
            </Button>
            <span className="px-2 text-xs tabular-nums">
              {pageIndex + 1} / {table.getPageCount() || 1}
            </span>
            <Button
              variant="outline"
              size="icon-sm"
              onClick={() => table.nextPage()}
              disabled={!canNext || isLoading}
              aria-label="Next page"
            >
              <ChevronRightIcon />
            </Button>
          </div>
        </div>
      )}
    </div>
  )
}

export type { ColumnDef, SortingState, PaginationState, ColumnFiltersState }
