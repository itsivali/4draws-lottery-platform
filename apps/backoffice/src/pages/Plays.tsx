import type { ColumnDef } from '@tanstack/react-table'
import { Search, Sparkles, Ticket } from 'lucide-react'
import { useMemo, useState } from 'react'
import { DataTable, Stack } from '../components/DataTable'
import { Empty, Loaded, Mono, PageHeader, Pill } from '../components/ui'
import { useSession } from '../context'
import { eat, kes, msisdn } from '../format'
import { usePages } from '../hooks'
import type { PlayRow } from '../types'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'

/** Every play, newest first, by player name and time (owner, 2026-10-30). */
export function PlaysPage() {
  const { api } = useSession()
  const [typed, setTyped] = useState('')
  const [q, setQ] = useState('')
  const [day, setDay] = useState('')
  const load = usePages((before, signal) =>
    api.get<{ plays: PlayRow[]; next_before: string | null }>('/v1/staff/plays',
      { limit: '50', ...(q ? { q } : {}), ...(day ? { day } : {}), ...(before === null ? {} : { before }) }, { signal })
      .then((r) => ({ rows: r.plays, next: r.next_before })), [q, day])

  const columns = useMemo<ColumnDef<PlayRow>[]>(() => [
    {
      id: 'when', header: 'When', accessorFn: (p) => p.created_at,
      cell: ({ row }) => <Stack primary={<span className="font-normal tabular-nums">{eat(row.original.created_at)}</span>} secondary={`play ${row.original.seq} of ${row.original.play_date}`} />,
    },
    {
      id: 'player', header: 'Player', accessorFn: (p) => `${p.full_name ?? ''} ${msisdn(p.msisdn)} ${p.user_id}`,
      cell: ({ row }) => (
        <Stack primary={row.original.full_name ?? <span className="font-normal text-muted-foreground">No name yet</span>}
          secondary={<span className="tabular-nums">{msisdn(row.original.msisdn)}</span>}>
          <Mono className="text-muted-foreground">{row.original.user_id}</Mono>
        </Stack>
      ),
    },
    {
      id: 'spin', header: 'Spin', accessorFn: (p) => p.slot ?? -1,
      cell: ({ row }) => {
        const p = row.original
        if (p.slot === null) return <span className="text-sm text-muted-foreground">—</span>
        return <div className="flex flex-wrap items-center gap-2"><span className="tabular-nums">Slot {p.slot}</span>{p.lucky ? <Pill tone="success"><Sparkles aria-hidden="true" className="size-3" /> Lucky</Pill> : null}</div>
      },
    },
    { id: 'price', header: 'Price', accessorFn: (p) => p.price_kes, meta: { className: 'text-right tabular-nums' }, cell: ({ row }) => kes(row.original.price_kes) },
    {
      id: 'status', header: 'Status', accessorFn: (p) => p.status,
      cell: ({ row }) => <Pill tone={row.original.status === 'paid' ? 'neutral' : 'warning'}><span className="capitalize">{row.original.status}</span></Pill>,
    },
  ], [])

  return (
    <section>
      <PageHeader title="Plays">
        Every play, newest first: who played, when, what it cost and where the wheel stopped. Names come from the
        player’s identity submission; each view is recorded in the audit log.
      </PageHeader>
      <form className="mb-4 flex flex-wrap items-center gap-3" onSubmit={(e) => { e.preventDefault(); setQ(typed.trim()) }}>
        <div className="relative w-full max-w-sm">
          <Search aria-hidden="true" className="pointer-events-none absolute left-2.5 top-1/2 size-4 -translate-y-1/2 text-muted-foreground" />
          <Input aria-label="Phone number or player id" placeholder="Phone number or player id" value={typed} onChange={(e) => { setTyped(e.target.value) }} className="pl-8" />
        </div>
        <Input type="date" aria-label="Play date" value={day} onChange={(e) => { setDay(e.target.value) }} className="w-auto" />
        <Button type="submit" variant="outline">Search</Button>
        {q || day ? <Button type="button" variant="ghost" onClick={() => { setTyped(''); setQ(''); setDay('') }}>Clear</Button> : null}
      </form>
      <Loaded load={load} what="plays">
        {(plays) =>
          plays.length === 0 ? <Empty icon={Ticket}>{q || day ? 'No play matches.' : 'No plays yet.'}</Empty> : (
            <DataTable columns={columns} data={plays} rowId={(p) => p.ticket_id} caption="Plays, newest first" filter="Filter these rows"
              more={{ has: load.more, loading: load.loadingMore, load: load.loadMore }} />
          )
        }
      </Loaded>
    </section>
  )
}
