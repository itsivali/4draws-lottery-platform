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
import { useState, type ReactNode } from 'react'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Table, TableBody, TableCaption, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'

/**
 * A sortable, filterable, paged table (TanStack Table on shadcn's Table). The caption names it for screen readers;
 * sorting is announced through aria-sort; filtering matches any cell's text.
 */
export function DataTable<T>({
  columns,
  data,
  caption,
  captionVisible = false,
  rowId,
  filter,
  pageSize = 25,
  initialSort = [],
  empty,
}: {
  columns: ColumnDef<T>[]
  data: T[]
  caption: string
  captionVisible?: boolean
  rowId: (row: T) => string
  /** placeholder of a search box over every column; omitted: no search */
  filter?: string
  pageSize?: number
  initialSort?: SortingState
  empty?: ReactNode
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
  return (
    <div className="mt-4 grid gap-3">
      {filter ? (
        <div className="relative max-w-sm">
          <Search aria-hidden="true" className="pointer-events-none absolute left-2.5 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
          <Input aria-label={filter} placeholder={filter} value={globalFilter} onChange={(e) => { setGlobalFilter(e.target.value) }} className="pl-8" />
        </div>
      ) : null}
      <div className="overflow-hidden rounded-xl border bg-card shadow-card">
        <Table>
          <TableCaption className={cn('mt-0 border-b px-4 py-3 text-left text-sm font-medium text-foreground caption-top', !captionVisible && 'sr-only')}>{caption}</TableCaption>
          <TableHeader className="bg-muted/50">
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
                      className={cn('h-10 text-xs font-medium uppercase tracking-wide text-muted-foreground', (h.column.columnDef.meta as ColumnMeta | undefined)?.className)}
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
                <TableRow key={row.id} className="align-top">
                  {row.getVisibleCells().map((cell) => (
                    <TableCell key={cell.id} className={cn('py-3', (cell.column.columnDef.meta as ColumnMeta | undefined)?.className)}>
                      {flexRender(cell.column.columnDef.cell, cell.getContext())}
                    </TableCell>
                  ))}
                </TableRow>
              ))
            )}
          </TableBody>
        </Table>
      </div>
      {pages > 1 ? (
        <nav aria-label={`${caption}: pages`} className="flex items-center justify-between gap-3 text-sm text-muted-foreground">
          <span>{total.toLocaleString('en-KE')} rows · page {table.getState().pagination.pageIndex + 1} of {pages}</span>
          <div className="flex gap-1">
            <Button type="button" variant="outline" size="sm" onClick={() => { table.previousPage() }} disabled={!table.getCanPreviousPage()}><ChevronLeft aria-hidden="true" /> Previous</Button>
            <Button type="button" variant="outline" size="sm" onClick={() => { table.nextPage() }} disabled={!table.getCanNextPage()}>Next <ChevronRight aria-hidden="true" /></Button>
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
