import {
  flexRender,
  getCoreRowModel,
  getFilteredRowModel,
  getPaginationRowModel,
  getSortedRowModel,
  useReactTable,
  type ColumnDef,
  type SortingState,
} from '@tanstack/react-table'
import { ArrowDown, ArrowUp, ArrowUpDown, ChevronLeft, ChevronRight, Search } from 'lucide-react'
import { useId, useState, type ReactNode } from 'react'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Table, TableBody, TableCaption, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'

/**
 * A sortable, filterable, paged table (TanStack Table on shadcn's Table), on a card with its title and a description
 * of what it lists (every table has one, 2026-11-11). The caption names it for screen readers; sorting is announced
 * through aria-sort; filtering matches any cell's text.
 */
export function DataTable<T>({
  columns,
  data,
  caption,
  description,
  rowId,
  filter,
  pageSize = 25,
  initialSort = [],
  empty,
  more,
  highlight,
}: {
  columns: ColumnDef<T>[]
  data: T[]
  /** the table's title, shown on its card and naming it for screen readers */
  caption: string
  /** what the table lists, in a sentence, under its title */
  description: string
  rowId: (row: T) => string
  /** placeholder of a search box over every column; omitted: no search */
  filter?: string
  pageSize?: number
  initialSort?: SortingState
  empty?: ReactNode
  /** the server has rows after these: a "Load more" control asks for them (omitted: every row is here) */
  more?: { has: boolean; loading: boolean; load: () => void }
  /** the row (by rowId) the reader was sent to, for example from an email: tinted and scrolled into view */
  highlight?: string | undefined
}) {
  const [sorting, setSorting] = useState<SortingState>(initialSort)
  const [globalFilter, setGlobalFilter] = useState('')
  const table = useReactTable({
    data,
    columns,
    state: { sorting, globalFilter },
    onSortingChange: setSorting,
    onGlobalFilterChange: setGlobalFilter,
    getRowId: rowId,
    getCoreRowModel: getCoreRowModel(),
    getSortedRowModel: getSortedRowModel(),
    getFilteredRowModel: getFilteredRowModel(),
    getPaginationRowModel: getPaginationRowModel(),
    initialState: { pagination: { pageSize } },
  })
  const rows = table.getRowModel().rows
  const pages = table.getPageCount()
  const total = table.getFilteredRowModel().rows.length
  const descId = useId()
  return (
    <div className="mt-4 grid gap-3 animate-rise-in">
      <div className="overflow-hidden rounded-3xl border bg-card shadow-card">
        <div className="flex flex-wrap items-end justify-between gap-3 px-5 pb-4 pt-5">
          <div className="min-w-0 max-w-[70ch]">
            <h3 className="m-0 font-display text-lg font-bold tracking-tight">{caption}</h3>
            <p id={descId} className="m-0 mt-1 text-[14px] leading-relaxed text-muted-foreground">{description}</p>
          </div>
          {filter ? (
            <div className="relative w-full max-w-xs">
              <Search aria-hidden="true" className="pointer-events-none absolute left-3.5 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
              <Input aria-label={filter} placeholder={filter} value={globalFilter} onChange={(e) => { setGlobalFilter(e.target.value) }} className="h-10 rounded-full bg-background pl-10" />
            </div>
          ) : null}
        </div>
        <Table aria-describedby={descId}>
          <TableCaption className="sr-only">{caption}</TableCaption>
          <TableHeader>
            {table.getHeaderGroups().map((hg) => (
              <TableRow key={hg.id} className="hover:bg-transparent">
                {hg.headers.map((h) => {
                  const sort = h.column.getIsSorted()
                  const sortable = h.column.getCanSort()
                  return (
                    <TableHead
                      key={h.id}
                      scope="col"
                      aria-sort={sort === 'asc' ? 'ascending' : sort === 'desc' ? 'descending' : undefined}
                      className={cn('h-11 border-y bg-muted/50 text-[12px] font-semibold uppercase tracking-wider text-muted-foreground first:pl-5 last:pr-5', (h.column.columnDef.meta as ColumnMeta | undefined)?.className)}
                    >
                      {h.isPlaceholder ? null : sortable ? (
                        <button
                          type="button"
                          className="-ml-2 inline-flex h-8 cursor-pointer items-center gap-1 rounded-md px-2 hover:bg-accent hover:text-accent-foreground"
                          onClick={h.column.getToggleSortingHandler()}
                        >
                          {flexRender(h.column.columnDef.header, h.getContext())}
                          {sort === 'asc' ? <ArrowUp aria-hidden="true" className="size-3.5" /> : sort === 'desc' ? <ArrowDown aria-hidden="true" className="size-3.5" /> : <ArrowUpDown aria-hidden="true" className="size-3.5 opacity-50" />}
                        </button>
                      ) : (
                        flexRender(h.column.columnDef.header, h.getContext())
                      )}
                    </TableHead>
                  )
                })}
              </TableRow>
            ))}
          </TableHeader>
          <TableBody>
            {rows.length === 0 ? (
              <TableRow className="hover:bg-transparent">
                <TableCell colSpan={columns.length} className="py-10 text-center text-muted-foreground">{empty ?? 'Nothing matches.'}</TableCell>
              </TableRow>
            ) : (
              rows.map((row) => (
                <TableRow
                  key={row.id}
                  className={cn('align-top transition-colors hover:bg-muted/40', row.id === highlight && 'bg-pending/10 hover:bg-pending/15')}
                  ref={row.id === highlight ? (el) => { el?.scrollIntoView({ block: 'center' }) } : undefined}
                >
                  {row.getVisibleCells().map((cell) => (
                    <TableCell key={cell.id} className={cn('py-3.5 text-[14.5px] first:pl-5 last:pr-5', (cell.column.columnDef.meta as ColumnMeta | undefined)?.className)}>
                      {flexRender(cell.column.columnDef.cell, cell.getContext())}
                    </TableCell>
                  ))}
                </TableRow>
              ))
            )}
          </TableBody>
        </Table>
      </div>
      {pages > 1 || more?.has || more?.loading ? (
        <nav aria-label={`${caption}: pages`} className="flex flex-wrap items-center justify-between gap-3 text-sm text-muted-foreground">
          <span aria-live="polite">
            {total.toLocaleString('en-KE')} rows{more?.has ? ' loaded' : ''}{pages > 1 ? ` · page ${table.getState().pagination.pageIndex + 1} of ${pages}` : ''}
          </span>
          <div className="flex flex-wrap gap-1">
            {more?.has || more?.loading ? (
              <Button type="button" variant="outline" size="sm" onClick={more.load} disabled={more.loading} aria-busy={more.loading}>
                {more.loading ? 'Loading…' : 'Load more'}
              </Button>
            ) : null}
            {pages > 1 ? (
              <>
                <Button type="button" variant="outline" size="sm" onClick={() => { table.previousPage() }} disabled={!table.getCanPreviousPage()}><ChevronLeft aria-hidden="true" /> Previous</Button>
                <Button type="button" variant="outline" size="sm" onClick={() => { table.nextPage() }} disabled={!table.getCanNextPage()}>Next <ChevronRight aria-hidden="true" /></Button>
              </>
            ) : null}
          </div>
        </nav>
      ) : null}
    </div>
  )
}

export interface ColumnMeta { className?: string }

/** A right-aligned action cell: buttons wrap on narrow screens. */
export function Actions({ children }: { children: ReactNode }) {
  return <div className="flex flex-wrap justify-end gap-2">{children}</div>
}

/** A primary line with a muted second line, the usual shape of a table cell here. */
export function Stack({ primary, secondary, children }: { primary: ReactNode; secondary?: ReactNode; children?: ReactNode }) {
  return (
    <div className="grid gap-0.5">
      <span className="font-medium">{primary}</span>
      {secondary ? <span className="text-sm text-muted-foreground">{secondary}</span> : null}
      {children}
    </div>
  )
}
